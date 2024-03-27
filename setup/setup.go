package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"crypto/rand"
	"encoding/base64"
	"encoding/json"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

func main() {

	b, err := os.ReadFile("../quickstart/credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}

	_ = getClient(config)
	reader := bufio.NewReader(os.Stdin)

	// Prompt user for credentials file path
	credentialsPath := "../quickstart/credentials.json"

	// Read contents of credentials.json file
	credentialsData, err := os.ReadFile(credentialsPath)
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// Prompt user for token file path

	tokenPath := "../quickstart/token.json"

	// Read contents of token.json file
	tokenData, err := os.ReadFile(tokenPath)
	if err != nil {
		log.Fatalf("Unable to read token file: %v", err)
	}

	// Prompt user for frequency input
	fmt.Print("Enter the frequency for the CronJob (in cron format, e.g., */5 * * * * for every 5 minutes): ")
	frequency, _ := reader.ReadString('\n')
	frequency = strings.TrimSpace(frequency)

	// Base64 encode credentials and token data
	credentialsBase64 := base64.StdEncoding.EncodeToString(credentialsData)
	tokenBase64 := base64.StdEncoding.EncodeToString(tokenData)

	// Generate a unique identifier for the CronJob
	cronJobName := fmt.Sprintf("my-cronjob-%d", time.Now().Unix())

	// Define the Secret YAML
	secretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: my-secret
type: Opaque
data:
  credentials.json: %s
  token.json: %s
`, credentialsBase64, tokenBase64)

	// Write the Secret YAML to a temporary file
	secretFile, err := os.CreateTemp("", "secret-*.yaml")
	if err != nil {
		log.Fatalf("Error in creating k8s cluster: %v", err)
	}
	defer os.Remove(secretFile.Name())

	if _, err := io.WriteString(secretFile, secretYAML); err != nil {
		log.Fatalf("Error in creating k8s cluster: %v", err)
	}
	if err := secretFile.Close(); err != nil {
		log.Fatalf("Error in creating k8s cluster: %v", err)
	}

	// Apply the Secrets YAML using kubectl
	applySecretCmd := exec.Command("kubectl", "apply", "-f", secretFile.Name())
	applySecretCmd.Stdout = os.Stdout
	applySecretCmd.Stderr = os.Stderr
	err = applySecretCmd.Run()
	if err != nil {
		log.Fatalf("Error in starting k8s cluster: %v", err)
	}

	// Define the CronJob YAML
	cronJobYAML := fmt.Sprintf(`
apiVersion: batch/v1
kind: CronJob
metadata:
  name: %s
spec:
  schedule: "%s"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: my-go-container
            image: aayushnagarr/drive-linux:1.2
            command: ["/app/quickstart-linux"]
            volumeMounts:
            - name: secret-volume
              mountPath: "/secrets"
              readOnly: true
            - name: backup-volume
              mountPath: "/app/backup"
          volumes:
          - name: secret-volume
            secret:
              secretName: my-secret
          - name: backup-volume
            persistentVolumeClaim:
              claimName: backup-pvc
          restartPolicy: OnFailure
`, cronJobName, frequency)

	// Write the CronJob YAML to a temporary file
	cronJobFile, err := os.CreateTemp("", "cronjob-*.yaml")
	if err != nil {
		log.Fatalf("Error in creating cronjob: %v", err)

	}
	defer os.Remove(cronJobFile.Name())

	if _, err := io.WriteString(cronJobFile, cronJobYAML); err != nil {
		log.Fatalf("Error in creating cronjob: %v", err)
	}
	if err := cronJobFile.Close(); err != nil {
		log.Fatalf("Error in creating cronjob: %v", err)
	}

	// Apply the CronJob YAML using kubectl
	applyCronJobCmd := exec.Command("kubectl", "apply", "-f", cronJobFile.Name())
	applyCronJobCmd.Stdout = os.Stdout
	applyCronJobCmd.Stderr = os.Stderr
	err = applyCronJobCmd.Run()
	if err != nil {
		log.Fatalf("Error in starting cronjob: %v", err)
	}

	fmt.Println("CronJob and Secret created successfully!")
}

func getClient(config *oauth2.Config) *http.Client {
	tokFile := "../quickstart/token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	} else {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Do you want to logout? (yes/no): ")
		text, _ := reader.ReadString('\n')
		if text == "yes\n" {
			os.Remove(tokFile)
			tok = getTokenFromWeb(config)
			saveToken(tokFile, tok)
		}
	}
	return config.Client(context.Background(), tok)
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	state := randToken()
	authURL := config.AuthCodeURL(state, oauth2.AccessTypeOffline)
	codeCh := make(chan string)

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.FormValue("state") != state {
				http.Error(w, "State invalid", http.StatusBadRequest)
				codeCh <- "State invalid"
				return
			}

			codeCh <- r.FormValue("code")
		})

		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	fmt.Printf("Please visit the following URL to authorize the application:\n%s\n", authURL)
	code := <-codeCh
	tok, err := config.Exchange(context.TODO(), code)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

func randToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func saveToken(path string, token *oauth2.Token) {
	//fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	t := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(t)
	defer f.Close()
	return t, err
}
