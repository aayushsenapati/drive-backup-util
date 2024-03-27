package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "os/exec"
    "path/filepath"
    "crypto/rand"
    "encoding/base64"
    "strings"
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

    client := getClient(config)

    srv, err := drive.New(client)
    if err != nil {
        log.Fatalf("Unable to retrieve Drive client: %v", err)
    }

    // Check if Git is initialized in the "backup" folder
    if _, err := os.Stat("backup/.git"); os.IsNotExist(err) {
        // Initialize Git in the "backup" folder
        cmd := exec.Command("git", "init")
        cmd.Dir = "backup"
        err = cmd.Run()
        if err != nil {
            log.Fatalf("Unable to initialize Git: %v", err)
        }
        
    }


    // Add all files to Git
    cmd := exec.Command("git", "add", "-A")
    cmd.Dir = "backup"
    err = cmd.Run()
    if err != nil {
        log.Fatalf("Unable to add files to Git: %v", err)
    }

    // Commit the changes
    cmd = exec.Command("git", "commit", "-m", "Backup commit")
    cmd.Dir = "backup"
    err = cmd.Run()
    if err != nil {
        log.Println("Unable to commit changes:", err)
    }


    // Get the latest commit hash
    cmd = exec.Command("git", "rev-parse", "HEAD")
    cmd.Dir = "backup"
    commitHashBytes, err := cmd.Output()
    if err != nil {
        log.Fatalf("Unable to get latest commit hash: %v", err)
    }
    commitHash := strings.TrimSpace(string(commitHashBytes))
    fmt.Println("Commit hash:", commitHash)


    //create backup folder if does not exist and get id

    fmt.Println("Creating backup folder if it does not exist")
    backupid:=getOrCreateFolder(srv, "backup","root")
    

    // Get the commit hash stored in Google Drive
    driveCommitHashFileId,err := getOrCreateHashFile(srv, "commit_hash.txt",backupid)
    if err != nil {
        log.Fatalf("Unable to get or create commit hash file in Google Drive: %v", err)
    }
    driveCommitHash, err := downloadFile(srv, driveCommitHashFileId)
    if err != nil {
        log.Fatalf("Unable to download commit hash file from Google Drive: %v", err)
    }
    driveCommitHash = strings.TrimSpace(string(driveCommitHash))
    fmt.Println("Drive commit hash:", driveCommitHash)

    // If the commit hashes are different, upload the changed files
    if commitHash != driveCommitHash {
        if driveCommitHash == "" {

            fmt.Println("No previous commit hash found. Uploading all files.")
            driveCommitHash = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

        }
        // Get the list of changed files
        cmd := exec.Command("git", "diff", "--diff-filter=AM", "--name-only", driveCommitHash, "HEAD")
        cmd.Dir = "backup"
        changedFilesBytes, err := cmd.Output()
        if err != nil {
            if exitError, ok := err.(*exec.ExitError); ok {
                fmt.Println("Differences found:", exitError)
            } else {
                log.Fatalf("Unable to get list of changed files: %v", err)
            }
        }

        changedFiles := strings.Split(string(changedFilesBytes), "\n")
        fmt.Println("Changed files:", changedFiles)

        for _, changedFile := range changedFiles {
            if changedFile != "" {
                fmt.Println("Changed file", changedFile)
                err = uploadDirectory(srv, changedFile, backupid)
                if err != nil {
                    log.Fatal(err)
                }
            }
        }


            // Get the list of deleted files and folders
        cmd = exec.Command("git", "diff", "--diff-filter=D", "--name-only", driveCommitHash, "HEAD")
        cmd.Dir = "backup"
        deletedPathsBytes, err := cmd.Output()
        if err != nil {
            if exitError, ok := err.(*exec.ExitError); ok {
                fmt.Println("Differences found:", exitError)
            } else {
                log.Fatalf("Unable to get list of deleted files and folders: %v", err)
            }
        }
        deletedPaths := strings.Split(string(deletedPathsBytes), "\n")
        fmt.Println("Deleted paths:", deletedPaths)

        // Delete the deleted files and folders from Google Drive
        for _, deletedPath := range deletedPaths {
            if deletedPath != "" {
                err = deletePath(srv,deletedPath,backupid)
                if err != nil {
                    log.Fatal(err)
                }
            }
        }





        // Update the commit hash stored in Google Drive
        err = updateFile(srv, driveCommitHashFileId, commitHash)
        if err != nil {
            log.Fatalf("Unable to update commit hash file in Google Drive: %v", err)
        }
    }
}

func uploadDirectory(srv *drive.Service, path string, parentID string) error {
    if strings.Contains(path, ".git") {
        return nil
    }
    fmt.Println("Uploading", path)
    wholePath := "backup" +string(filepath.Separator)+path

    // Split the file path into directories and the file name
    directories, fileName := filepath.Split(path)

    // Create the directories in Google Drive if they don't exist
    directoryIDs := strings.Split(directories, string(filepath.Separator))
    for _, directory := range directoryIDs {
        if directory != "" {
            parentID = getOrCreateFolder(srv, directory, parentID)
        }
    }

    // Check if the file exists
    fileList, err := srv.Files.List().Q(fmt.Sprintf("name='%s' and '%s' in parents", fileName, parentID)).Do()
    if err != nil {
        fmt.Println("Unable to retrieve files:", err)
    }

    if fileList != nil && len(fileList.Files) > 0 {
        // The file exists, update it
        fileId := fileList.Files[0].Id
        file, err := os.Open(wholePath)
        if err != nil {
            log.Fatalf("Unable to open file: %v", err)
        }
        _, err = srv.Files.Update(fileId, &drive.File{}).Media(file).Do()
        if err != nil {
            log.Fatalf("Unable to update file: %v", err)
        }
    } else {
        // The file doesn't exist, upload it
        uploadFile(srv, wholePath, parentID)
    }

    return nil
}

func getOrCreateHashFile(srv *drive.Service, fileName string, parentID string) (string, error) {
    // Search for the file in the specified parent directory
    r, err := srv.Files.List().Q(fmt.Sprintf("name='%s' and mimeType='text/plain' and '%s' in parents", fileName, parentID)).Do()
    if err != nil {
        return "", err
    }

    // If the file exists, return its ID
    if len(r.Files) > 0 {
        return r.Files[0].Id, nil
    }

    // If the file doesn't exist, create it in the specified parent directory
    file, err := srv.Files.Create(&drive.File{
        Name:     fileName,
        Parents:  []string{parentID},
        MimeType: "text/plain",
    }).Do()
    if err != nil {
        return "", err
    }

    // Return the ID of the newly created file
    return file.Id, nil
}

func deletePath(srv *drive.Service, path string, backupFolderID string) error {
    // Split the path into its components
    pathComponents := strings.Split(path, "/")

    // Start from the backup folder
    currentFolderID := backupFolderID
    parentFolderIDs := []string{}

    // Traverse the path
    for _, component := range pathComponents {
        // Find the file or folder with the current name
        files, err := srv.Files.List().Q(fmt.Sprintf("name='%s' and '%s' in parents", component, currentFolderID)).Do()
        if err != nil {
            return err
        }

        // If a matching file or folder is not found, return an error
        if len(files.Files) == 0 {
            return fmt.Errorf("file or folder not found: %s", component)
        }

        // Move to the next folder in the path
        parentFolderIDs = append(parentFolderIDs, currentFolderID)
        currentFolderID = files.Files[0].Id
    }

    // Delete the file or folder
    err := srv.Files.Delete(currentFolderID).Do()
    if err != nil {
        return err
    }

    // Check each parent folder and delete it if it's empty
    for i := len(parentFolderIDs) - 1; i >= 0; i-- {
        files, err := srv.Files.List().Q(fmt.Sprintf("'%s' in parents", parentFolderIDs[i])).Do()
        if err != nil {
            return err
        }
        if len(files.Files) == 0 {
            err = srv.Files.Delete(parentFolderIDs[i]).Do()
            if err != nil {
                return err
            }
        } else {
            // If a folder is not empty, stop checking
            break
        }
    }

    return nil
}

func getClient(config *oauth2.Config) *http.Client {
    tokFile := "token.json"
    tok, err := tokenFromFile(tokFile)
    if err != nil {
        log.Fatalf("Unable to retrieve token from file: %v", err)
    } 
    return config.Client(context.Background(), tok)
    }

func uploadFile(srv *drive.Service, path string, folderId string) {

    fmt.Println("Uploading", path, "to", folderId)
    fmt.Println("this is base",filepath.Base(path))
    f := &drive.File{
        Name:     filepath.Base(path),
        Parents:  []string{folderId},
        MimeType: "application/octet-stream",
    }
    //fmt.Println("Uploading", path, "to", folderId)

    file, err := os.Open(path)
    if err != nil {
        log.Fatalf("error opening %q: %v", path, err)
    }
    defer file.Close()

    _, err = srv.Files.Create(f).Media(file).Do()
    if err != nil {
        log.Fatalf("error creating %q: %v", path, err)
    }
}

func downloadFile(srv *drive.Service, fileId string) (string, error) {
    file, err := srv.Files.Get(fileId).Download()
    if err != nil {
        return "", err
    }
    defer file.Body.Close()

    bytes, err := ioutil.ReadAll(file.Body)
    if err != nil {
        return "", err
    }

    return string(bytes), nil
}

func updateFile(srv *drive.Service, fileId, newContent string) error {
    // Create a new file with the new content
    newFile, err := ioutil.TempFile("", "drive")
    if err != nil {
        return err
    }
    defer os.Remove(newFile.Name())

    // Write the new content to the new file
    _, err = newFile.WriteString(newContent)
    if err != nil {
        return err
    }

    // Close the new file
    err = newFile.Close()
    if err != nil {
        return err
    }

    // Open the new file for reading
    f, err := os.Open(newFile.Name())
    if err != nil {
        return err
    }
    defer f.Close()

    // Update the file in Google Drive with the new file
    _, err = srv.Files.Update(fileId, nil).Media(f).Do()
    if err != nil {
        return err
    }

    return nil
}

func getOrCreateFolder(srv *drive.Service, folderName string, parentID string) string {
    fmt.Println("Checking if folder exists")
    fmt.Println("folder name",folderName)
    q := fmt.Sprintf("name='%s' and mimeType='application/vnd.google-apps.folder' and '%s' in parents", folderName, parentID)
    r, err := srv.Files.List().Q(q).Do()
    if err != nil {
        log.Fatalf("Unable to retrieve files: %v", err)
    }

    if len(r.Files) > 0 {
        return r.Files[0].Id
    }

    f := &drive.File{
        Name:     folderName,
        MimeType: "application/vnd.google-apps.folder",
        Parents:  []string{parentID},
    }
    folder, err := srv.Files.Create(f).Do()
    if err != nil {
        log.Fatalf("Unable to create directory: %v", err)
    }

    return folder.Id
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