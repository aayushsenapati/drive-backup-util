package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
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
            image: aayushnagarr/dri:latest
            command: ["/app/quickstart.exe"]  # Change this to the path of your Go script
            volumeMounts:
            - name: secret-volume
              mountPath: "/secrets"
              readOnly: true
          volumes:
          - name: secret-volume
            secret:
              secretName: my-secret
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
