package main

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"

    "crypto/rand"
    "encoding/base64"

    "os"

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

    }




func getClient(config *oauth2.Config) *http.Client {
    tokFile := "token.json"
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