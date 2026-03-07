package sites

import (
	"fmt"
	"os"
	"os/exec"
)

// PHPInfo holds info about an installed PHP version
type PHPInfo struct {
	Version   string `json:"version"`
	FPMSocket string `json:"fpm_socket"`
	Active    bool   `json:"active"`
}

// InstallPHP installs a specific PHP version via apt
func InstallPHP(version string) error {
	packages := []string{
		fmt.Sprintf("php%s-fpm", version),
		fmt.Sprintf("php%s-cli", version),
		fmt.Sprintf("php%s-common", version),
		fmt.Sprintf("php%s-mysql", version),
		fmt.Sprintf("php%s-zip", version),
		fmt.Sprintf("php%s-gd", version),
		fmt.Sprintf("php%s-mbstring", version),
		fmt.Sprintf("php%s-curl", version),
		fmt.Sprintf("php%s-xml", version),
		fmt.Sprintf("php%s-bcmath", version),
		fmt.Sprintf("php%s-intl", version),
		fmt.Sprintf("php%s-readline", version),
		fmt.Sprintf("php%s-redis", version),
		fmt.Sprintf("php%s-sqlite3", version),
	}

	args := append([]string{"install", "-y"}, packages...)
	cmd := exec.Command("apt-get", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install PHP %s: %w", version, err)
	}

	// Enable and start FPM
	svc := fmt.Sprintf("php%s-fpm", version)
	exec.Command("systemctl", "enable", svc).Run()
	exec.Command("systemctl", "start", svc).Run()

	return nil
}

// GetInstalledPHP returns details of all installed PHP versions
func GetInstalledPHP() []PHPInfo {
	versions := GetAvailablePHPVersions()
	var result []PHPInfo

	for _, v := range versions {
		socket := fmt.Sprintf("/var/run/php/php%s-fpm.sock", v)
		_, socketExists := os.Stat(socket)

		result = append(result, PHPInfo{
			Version:   v,
			FPMSocket: socket,
			Active:    socketExists == nil,
		})
	}

	return result
}
