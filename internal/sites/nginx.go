package sites

import "fmt"

// GenerateNginxVhost creates an Nginx server block config for a PHP site
func GenerateNginxVhost(domain, docRoot, phpVersion string, port int) string {
	phpSocket := fmt.Sprintf("/var/run/php/php%s-fpm.sock", phpVersion)

	return fmt.Sprintf(`# TunnelPanel managed — %s
# Do not edit manually, changes will be overwritten

server {
    listen %d;
    server_name %s;
    root %s;
    index index.php index.html index.htm;

    # Logging
    access_log /var/log/nginx/%s-access.log;
    error_log  /var/log/nginx/%s-error.log;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # Max upload size
    client_max_body_size 100M;

    # Main location
    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }

    # PHP-FPM
    location ~ \.php$ {
        fastcgi_pass unix:%s;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include fastcgi_params;
        fastcgi_index index.php;
        fastcgi_read_timeout 300;
    }

    # Deny dotfiles (except .well-known)
    location ~ /\.(?!well-known) {
        deny all;
    }

    # Static file caching
    location ~* \.(jpg|jpeg|png|gif|ico|css|js|woff2?|ttf|svg)$ {
        expires 30d;
        add_header Cache-Control "public, immutable";
    }
}
`, domain, port, domain, docRoot, domain, domain, phpSocket)
}

// GenerateNginxProxy creates an Nginx reverse proxy config (for containers, Node apps, etc.)
func GenerateNginxProxy(domain string, targetPort, listenPort int) string {
	return fmt.Sprintf(`# TunnelPanel managed — %s (proxy)

server {
    listen %d;
    server_name %s;

    location / {
        proxy_pass http://127.0.0.1:%d;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 86400;
    }
}
`, domain, listenPort, domain, targetPort)
}
