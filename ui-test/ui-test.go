package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"crypto/rand"
	"encoding/base64"

	"github.com/rivo/tview"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

var (
	app        *tview.Application
	cronFreq   int
	filePath   string
	logout     bool
	config     *oauth2.Config
)

func main() {
	b, err := os.ReadFile("../config/credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}

	_ = getClient(config)
	app = tview.NewApplication()

	form := tview.NewForm()

	form.AddInputField("Cronjob Frequency (minutes)", "", 0, nil, func(text string) {
		cronFreq, _ = strconv.Atoi(text)
	}).
		AddInputField("File Path", "", 0, nil, func(text string) {
			filePath = text
		}).
		AddButton("Re-Login", func() {
			authURL := getAuthURL()
			modal := tview.NewModal().
				SetText(fmt.Sprintf("Authentication URL:\n%s", authURL)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					app.SetRoot(form, true)
				})
			app.SetRoot(modal, true)
		}).
		AddButton("Save", func() {
			saveConfiguration()
		}).
		AddButton("Quit", func() {
			app.Stop()
		})

	form.SetBorder(true).SetTitle("Drive Settings").SetTitleAlign(tview.AlignCenter)

	if err := app.SetRoot(form, true).Run(); err != nil {
		panic(err)
	}
}

func getAuthURL() string {
	// Your authentication URL generation logic here

		os.Remove("../config/token.json")

		// Re-run login to get new token
		getClient(config)
		// Update Kubernetes secret
		err := updateKubernetesSecret("../config/token.json")
		if err != nil {
			log.Fatalf("Failed to update Kubernetes secret: %v", err)
		}

	return "https://example.com/auth"
}

func saveConfiguration() {
	b, err := os.ReadFile("../config/credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	config, err = google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}

	getClient(config)

	// Prompt for cron job frequency
	fmt.Print("Enter the frequency of the cron job in minutes: ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	cronFreq, err := strconv.Atoi(input)
	if err != nil {
		log.Fatalf("Invalid input: %v", err)
	}

	// Prompt for backup folder directory
	var dir string
	if runtime.GOOS == "linux" {
		fmt.Print("Enter backup folder dir relative to minikube mount (/host)")
	} else if runtime.GOOS == "windows" {
		fmt.Print("Enter backup folder dir relative c drive (/c/Users/...)")
	} else {
		fmt.Print("Enter backup folder dir relative to kubernetes root")
	}

	reader = bufio.NewReader(os.Stdin)
	dir, _ = reader.ReadString('\n')
	dir = strings.TrimSpace(dir)
	if err != nil {
		log.Fatalf("Invalid input: %v", err)
	}

	// Generate CronJob YAML configuration
	cronJobYAML := generateCronJobYAML(cronFreq)

	// Generate PVC YAML configuration
	pvcYaml := generatePvcYAML(dir)

	// Read deployment YAML
	deploy, err := os.ReadFile("../config/deployment.yml")
	if err != nil {
		log.Fatalf("Unable to read deployment YAML: %v", err)
	}
	deploymentYAML := string(deploy)

	// Apply CronJob YAML to Kubernetes deployment
	applyYAML(deploymentYAML, cronJobYAML, pvcYaml)

	// Check if user wants to logout
	fmt.Print("Do you want to logout? (yes/no): ")
	text, _ := reader.ReadString('\n')
	if strings.TrimSpace(text) == "yes" {
		// Remove existing token
		os.Remove("../config/token.json")

		// Re-run login to get new token
		getClient(config)

		// Update Kubernetes secret
		err = updateKubernetesSecret("../config/token.json")
		if err != nil {
			log.Fatalf("Failed to update Kubernetes secret: %v", err)
		}
	}
	fmt.Println("Setup complete!")
}

func getClient(config *oauth2.Config) *http.Client {
	tokFile := "../config/token.json"
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
			// send test response
			w.Write([]byte("Authorization successful! You can close this tab now."))

			codeCh <- r.FormValue("code")
		})

		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	fmt.Println("Opening browser to visit the following URL:")
	if runtime.GOOS == "linux" {
		fmt.Print("In linux login")
		cmd := exec.Command("xdg-open", authURL)
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Unable to open browser: %v, output: %s", err, output)
			fmt.Printf("Please visit the following URL to authorize the application:\n%s\n", authURL)
		}
	} else if runtime.GOOS == "windows" {
		// fmt.Println("MY AUTH URL IS : ", authURL)
		// cmd := exec.Command("cmd", "/c", "start", "", authURL)
		// cmd := exec.Command("cmd", "/c", "start", authURL)
		// err := cmd.Start()
		// if err != nil {
		// fmt.Printf("Unable to open browser: %v", err)
		fmt.Printf("Please visit the following URL to authorize the application:\n%s\n", authURL)
		// }
	}
	// fmt.Printf("Please visit the following URL to authorize the application:\n%s\n", authURL)

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
	cronTemplate, err := os.ReadFile("../config/cron.yml")
	if err != nil {
		log.Fatalf("Unable to read cron job template: %v", err)
	}
	return fmt.Sprintf(string(cronTemplate), minutes)

}

func generatePvcYAML(dir string) string {
	pvcTemplate, err := os.ReadFile("../config/pvc.yml")
	if err != nil {
		log.Fatalf("Unable to read cron job template: %v", err)
	}
	if runtime.GOOS == "linux" {
		dir = "/host/" + dir
	} else if runtime.GOOS == "windows" {
		dir = "/run/desktop/mnt/host/" + dir
	}
	return fmt.Sprintf(string(pvcTemplate), dir)

}

func applyYAML(deployYaml, cronYaml, pvcYaml string) {
	// delete deployment if it already exists
	cmd := exec.Command("kubectl", "delete", "deployment", "drive-backup-deployment")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("error deleting deployent: %v, output: %s", err, output)
	}
	// delete cronjob if it already exists
	cmd = exec.Command("kubectl", "delete", "cronjob", "drive-backup-cronjob")
	output, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("error deleting cronjob: %v, output: %s", err, output)
	}
	// delete pv and pvc if they already exist
	cmd = exec.Command("kubectl", "delete", "pvc", "backup-pvc")
	output, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("error deleting pvc: %v, output: %s", err, output)
	}
	cmd = exec.Command("kubectl", "delete", "pv", "backup-pv")
	output, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("error deleting pv: %v, output: %s", err, output)
	}

	// create deployment
	cmd = exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(deployYaml)
	output, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("error applying deploy YAML: %v, output: %s", err, output)
	}
	// create pvc
	cmd = exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(pvcYaml)
	output, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("error applying pvc YAML: %v, output: %s", err, output)
	}
	// create cronjob
	cmd = exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(cronYaml)
	output, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("error applying cron YAML: %v, output: %s", err, output)
	}
}

func updateKubernetesSecret(tokenFile string) error {
	cmdDelete := exec.Command("kubectl", "delete", "secret", "token")
	if err := cmdDelete.Run(); err != nil {
		fmt.Printf("error deleting existing secret: %v", err)
	}

	fileFlag := fmt.Sprintf("--from-file=%s", tokenFile)
	cmdCreate := exec.Command("kubectl", "create", "secret", "generic", "token", fileFlag)
	if err := cmdCreate.Run(); err != nil {
		return fmt.Errorf("error creating new secret: %v", err)
	}
	return nil
}
