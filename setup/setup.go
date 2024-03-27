package main

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "os/exec"
    "strconv"
    "strings"

    "crypto/rand"
    "encoding/base64"

    "golang.org/x/oauth2"
    "golang.org/x/oauth2/google"
    "google.golang.org/api/drive/v3"
)

func main() {
    b, err := ioutil.ReadFile("credentials.json")
    if err != nil {
        log.Fatalf("Unable to read client secret file: %v", err)
    }

    config, err := google.ConfigFromJSON(b, drive.DriveScope)
    if err != nil {
        log.Fatalf("Unable to parse client secret file to config: %v", err)
    }

    _ = getClient(config)
    // Prompt for cron job frequency
    fmt.Print("Enter the frequency of the cron job in minutes: ")
    reader := bufio.NewReader(os.Stdin)
    input, _ := reader.ReadString('\n')
    input = strings.TrimSpace(input)
    minutes, err := strconv.Atoi(input)
    if err != nil {
        log.Fatalf("Invalid input: %v", err)
    }

    // Generate CronJob YAML configuration
    cronJobYAML := generateCronJobYAML(minutes)

    // Apply CronJob YAML to Kubernetes deployment
    err = applyCronJobYAML(cronJobYAML)
    if err != nil {
        log.Fatalf("Failed to apply CronJob YAML: %v", err)
    }

    // Check if user wants to logout
    fmt.Print("Do you want to logout? (yes/no): ")
    text, _ := reader.ReadString('\n')
    if text == "yes\n" {
        // Remove existing token
        os.Remove("token.json")
        // Re-run login to get new token
        getClient(config)
        // Update Kubernetes secret
        err = updateKubernetesSecret("token.json")
        if err != nil {
            log.Fatalf("Failed to update Kubernetes secret: %v", err)
        }
    }
}

func getClient(config *oauth2.Config) *http.Client {
    tokFile := "token.json"
    tok, err := tokenFromFile(tokFile)
    if err != nil {
        tok = getTokenFromWeb(config)
        saveToken(tokFile, tok)
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

func generateCronJobYAML(minutes int) string {
    frequency := fmt.Sprintf("*/%d * * * *", minutes)
    return fmt.Sprintf(`apiVersion: v1
kind: CronJob
metadata:
  name: drive-backup-cronjob
spec:
  schedule: "%s"
  jobTemplate:
    spec:
      template:
        metadata:
          labels:
            app: drive-backup
        spec:
          containers:
          - name: drive-backup-container
            image: aayushsenapati/drive-backup:latest
            volumeMounts:
            - name: google-credentials
              mountPath: /app/credentials.json
              subPath: credentials.json
            - name: backup
              mountPath: /app/backup
            - name: token
              mountPath: /app/token.json
              subPath: token.json
          volumes:
          - name: google-credentials
            secret:
              secretName: google-credentials
          - name: backup
            persistentVolumeClaim:
              claimName: backup-pvc
          - name: token
            secret:
              secretName: token
          restartPolicy: OnFailure
`, frequency)
}

func applyCronJobYAML(yaml string) error {
    cmd := exec.Command("kubectl", "apply", "-f", "-")
    cmd.Stdin = strings.NewReader(yaml)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("error applying YAML: %v, output: %s", err, output)
    }
    return nil
}

func updateKubernetesSecret(tokenFile string) error {
    cmdDelete := exec.Command("kubectl", "delete", "secret", "token")
    if err := cmdDelete.Run(); err != nil {
        return fmt.Errorf("error deleting existing secret: %v", err)
    }

    cmdCreate := exec.Command("kubectl", "create", "secret", "generic", "token", "--from-file=token.json")
    if err := cmdCreate.Run(); err != nil {
        return fmt.Errorf("error creating new secret: %v", err)
    }
    return nil
}
