FROM nginx:alpine

# Copy static files
COPY index.html /usr/share/nginx/html/
COPY package.json /usr/share/nginx/html/

# Copy nginx configuration
COPY <<EOF /etc/nginx/conf.d/default.conf
server {
    listen 80;
    server_name localhost;
    root /usr/share/nginx/html;
    index index.html;

    # Handle SPA routing
    location / {
        try_files \$uri \$uri/ /index.html;
    }

    # Enable gzip compression
    gzip on;
    gzip_types text/css application/javascript text/javascript;

    # Security headers
    add_header X-Frame-Options DENY;
    add_header X-Content-Type-Options nosniff;
}
EOF

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
