package http

import (
	_ "embed"
	"net/http"
)

//go:embed swagger/openapi.yaml
var swaggerSpec []byte

func HandleSwaggerSpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, _ = w.Write(swaggerSpec)
}

func HandleSwaggerUI(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Crypto Price Aggregator - API Docs</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
        SwaggerUIBundle({
            url: '/swagger/openapi.yaml',
            dom_id: '#swagger-ui',
        });
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(html))
}
