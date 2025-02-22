package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Read private key from file
func parsePrivateKey(privateKeyPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing private key")
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse RSA private key: %w", err)
		}
	}
	return privateKey.(*rsa.PrivateKey), nil
}

// Generate a JWT for the GitHub App authentication
func generateGitHubAppJWT(appID string, privateKey *rsa.PrivateKey) (string, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iat": now.Unix(),                       // Issued at time
		"exp": now.Add(time.Minute * 10).Unix(), // Expires in 10 minutes
		"iss": appID,                            // GitHub App ID
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(privateKey)
}

// Fetch the installation access token from GitHub
func getInstallationAccessToken(appJWT, installationID string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/app/installations/%s/access_tokens", installationID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API error: %s, response: %s", resp.Status, string(body))
	}

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	token, ok := result["token"].(string)
	if !ok {
		return "", fmt.Errorf("failed to retrieve access token from response")
	}
	return token, nil
}

func main() {
	// Command-line arguments
	appID := flag.String("appId", "", "GitHub App ID")
	installationID := flag.String("appInstallationId", "", "GitHub App Installation ID")
	privateKeyPath := flag.String("privateKey", "", "Path to private key file")

	flag.Parse()

	// Validate required parameters
	if *appID == "" || *installationID == "" || *privateKeyPath == "" {
		log.Fatal("Error: Missing required parameters. Usage: ./fetch_github_token -appId=<APP_ID> -appInstallationId=<INSTALLATION_ID> -privateKey=<PRIVATE_KEY_PATH>")
	}

	// Read private key file
	privateKeyBytes, err := os.ReadFile(*privateKeyPath)
	if err != nil {
		log.Fatalf("Failed to read private key file: %v", err)
	}
	privateKey, err := parsePrivateKey(string(privateKeyBytes))
	if err != nil {
		log.Fatalf("Failed to parse private key: %v", err)
	}

	// Generate JWT
	appJWT, err := generateGitHubAppJWT(*appID, privateKey)
	if err != nil {
		log.Fatalf("Failed to generate GitHub App JWT: %v", err)
	}

	// Fetch installation access token
	_, err = getInstallationAccessToken(appJWT, *installationID)
	if err != nil {
		log.Fatalf("Failed to get installation access token: %v", err)
	}

	fmt.Printf("GitHub App Access Token: %s\n", appJWT)
}
