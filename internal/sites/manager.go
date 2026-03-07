package sites

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Muhammedhashirm009/tunnel-panel/internal/database"
	"github.com/Muhammedhashirm009/tunnel-panel/internal/tunnel"
)

// Site represents a hosted PHP website
type Site struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	Domain          string    `json:"domain"`
	DocumentRoot    string    `json:"document_root"`
	PHPVersion      string    `json:"php_version"`
	Port            int       `json:"port"`
	NginxConfigPath string    `json:"nginx_config_path"`
	DNSRecordID     string    `json:"dns_record_id"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

// Manager handles site lifecycle operations
type Manager struct {
	tunnelMgr   *tunnel.Manager
	sitesRoot   string // e.g., /var/www
	nginxConf   string // e.g., /etc/nginx/sites-enabled
}

// NewManager creates a new site manager
func NewManager(tunnelMgr *tunnel.Manager) *Manager {
	return &Manager{
		tunnelMgr: tunnelMgr,
		sitesRoot: "/var/www",
		nginxConf: "/etc/nginx/sites-enabled",
	}
}

// CreateSite provisions a new PHP site: creates dirs, nginx vhost, PHP-FPM pool, tunnel ingress
func (m *Manager) CreateSite(name, domain, phpVersion string) (*Site, error) {
	// Validate
	if name == "" || domain == "" {
		return nil, fmt.Errorf("name and domain are required")
	}
	if phpVersion == "" {
		phpVersion = "8.2"
	}

	// Check if domain already exists
	var count int
	database.DB().QueryRow("SELECT COUNT(*) FROM sites WHERE domain = ?", domain).Scan(&count)
	if count > 0 {
		return nil, fmt.Errorf("domain %s already exists", domain)
	}

	// Allocate port
	port, err := m.tunnelMgr.AllocatePort("site", 0) // app_id will be set after insert
	if err != nil {
		return nil, fmt.Errorf("port allocation failed: %w", err)
	}

	// Create document root
	docRoot := filepath.Join(m.sitesRoot, name)
	if err := os.MkdirAll(docRoot, 0755); err != nil {
		return nil, fmt.Errorf("cannot create doc root: %w", err)
	}

	// Create default index.php
	indexContent := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>%s</title></head>
<body>
<h1>Welcome to %s</h1>
<p>Domain: %s</p>
<p>PHP Version: <?php echo phpversion(); ?></p>
<p>Managed by TunnelPanel</p>
</body>
</html>`, name, name, domain)
	os.WriteFile(filepath.Join(docRoot, "index.php"), []byte(indexContent), 0644)

	// Generate Nginx vhost
	confPath := filepath.Join(m.nginxConf, name+".conf")
	vhostContent := GenerateNginxVhost(domain, docRoot, phpVersion, port)
	if err := os.WriteFile(confPath, []byte(vhostContent), 0644); err != nil {
		return nil, fmt.Errorf("cannot write nginx config: %w", err)
	}

	// Set ownership
	exec.Command("chown", "-R", "www-data:www-data", docRoot).Run()

	// Insert into database
	result, err := database.DB().Exec(
		`INSERT INTO sites (name, domain, document_root, php_version, port, nginx_config_path, status, created_at) 
		 VALUES (?, ?, ?, ?, ?, ?, 'active', ?)`,
		name, domain, docRoot, phpVersion, port, confPath, time.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	siteID, _ := result.LastInsertId()

	// Update port allocation with actual site ID
	database.DB().Exec("UPDATE ports SET app_id = ? WHERE port = ?", siteID, port)

	// Add tunnel ingress rule
	if m.tunnelMgr != nil {
		m.tunnelMgr.AddIngressRule(domain, port, "site", int(siteID))
	}

	// Reload Nginx
	ReloadNginx()

	return &Site{
		ID:              int(siteID),
		Name:            name,
		Domain:          domain,
		DocumentRoot:    docRoot,
		PHPVersion:      phpVersion,
		Port:            port,
		NginxConfigPath: confPath,
		Status:          "active",
		CreatedAt:       time.Now(),
	}, nil
}

// ListSites returns all sites from the database
func (m *Manager) ListSites() ([]Site, error) {
	rows, err := database.DB().Query(
		"SELECT id, name, domain, document_root, php_version, port, nginx_config_path, COALESCE(dns_record_id,''), status, created_at FROM sites ORDER BY id DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sites []Site
	for rows.Next() {
		var s Site
		if err := rows.Scan(&s.ID, &s.Name, &s.Domain, &s.DocumentRoot, &s.PHPVersion, &s.Port, &s.NginxConfigPath, &s.DNSRecordID, &s.Status, &s.CreatedAt); err != nil {
			continue
		}
		sites = append(sites, s)
	}
	return sites, nil
}

// GetSite returns a single site by ID
func (m *Manager) GetSite(id int) (*Site, error) {
	var s Site
	err := database.DB().QueryRow(
		"SELECT id, name, domain, document_root, php_version, port, nginx_config_path, COALESCE(dns_record_id,''), status, created_at FROM sites WHERE id = ?", id,
	).Scan(&s.ID, &s.Name, &s.Domain, &s.DocumentRoot, &s.PHPVersion, &s.Port, &s.NginxConfigPath, &s.DNSRecordID, &s.Status, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("site not found")
	}
	return &s, nil
}

// DeleteSite removes a site completely
func (m *Manager) DeleteSite(id int) error {
	site, err := m.GetSite(id)
	if err != nil {
		return err
	}

	// Remove tunnel ingress
	if m.tunnelMgr != nil {
		m.tunnelMgr.RemoveIngressRule(site.Domain)
	}

	// Remove Nginx config
	os.Remove(site.NginxConfigPath)

	// Remove document root (optional — could be dangerous)
	// os.RemoveAll(site.DocumentRoot)

	// Free port
	database.DB().Exec("DELETE FROM ports WHERE port = ?", site.Port)

	// Delete from DB
	database.DB().Exec("DELETE FROM sites WHERE id = ?", id)

	// Reload Nginx
	ReloadNginx()

	return nil
}

// UpdatePHPVersion changes the PHP version for a site
func (m *Manager) UpdatePHPVersion(id int, newVersion string) error {
	site, err := m.GetSite(id)
	if err != nil {
		return err
	}

	// Regenerate nginx config
	vhostContent := GenerateNginxVhost(site.Domain, site.DocumentRoot, newVersion, site.Port)
	if err := os.WriteFile(site.NginxConfigPath, []byte(vhostContent), 0644); err != nil {
		return err
	}

	// Update DB
	database.DB().Exec("UPDATE sites SET php_version = ? WHERE id = ?", newVersion, id)

	ReloadNginx()
	return nil
}

// GetAvailablePHPVersions returns installed PHP versions
func GetAvailablePHPVersions() []string {
	versions := []string{}
	candidates := []string{"7.4", "8.0", "8.1", "8.2", "8.3"}
	for _, v := range candidates {
		socket := fmt.Sprintf("/var/run/php/php%s-fpm.sock", v)
		if _, err := os.Stat(socket); err == nil {
			versions = append(versions, v)
		}
		// Also check if the binary exists
		bin := fmt.Sprintf("/usr/bin/php%s", v)
		if _, err := os.Stat(bin); err == nil {
			found := false
			for _, existing := range versions {
				if existing == v {
					found = true
					break
				}
			}
			if !found {
				versions = append(versions, v)
			}
		}
	}
	if len(versions) == 0 {
		versions = append(versions, "8.2") // fallback
	}
	return versions
}

// ReloadNginx tests and reloads nginx config
func ReloadNginx() error {
	// Test config first
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx config test failed: %s", strings.TrimSpace(string(out)))
	}
	return exec.Command("systemctl", "reload", "nginx").Run()
}
