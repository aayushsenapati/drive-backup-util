package diff

import (
	// "fmt"
	"io"
	// "os"

	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)


func GetModifiedFiles(repodir, commitOld, commitNew string) []string {
	repo, err := git.PlainOpen(repodir)
	if err != nil {
		panic(err)
	}

	commit1, err := repo.CommitObject(plumbing.NewHash(commitOld))
	if err != nil {
		panic(err)
	}

	commit2, err := repo.CommitObject(plumbing.NewHash(commitNew))
	if err != nil {
		panic(err)
	}

	tree1, err := commit1.Tree()
	if err != nil {
		panic(err)
	}

	tree2, err := commit2.Tree()
	if err != nil {
		panic(err)
	}

	_, _, modifiedFiles := compareFiles(repo, tree1, tree2)
	return modifiedFiles
}

func GetAppendedFiles(repodir, commitOld, commitNew string) []string {
	repo, err := git.PlainOpen(repodir)
	if err != nil {
		panic(err)
	}

	commit1, err := repo.CommitObject(plumbing.NewHash(commitOld))
	if err != nil {
		panic(err)
	}

	commit2, err := repo.CommitObject(plumbing.NewHash(commitNew))
	if err != nil {
		panic(err)
	}

	tree1, err := commit1.Tree()
	if err != nil {
		panic(err)
	}

	tree2, err := commit2.Tree()
	if err != nil {
		panic(err)
	}

	_, appendedFiles, _ := compareFiles(repo, tree1, tree2)
	return appendedFiles
}

func GetDeletedFiles(repodir, commitOld, commitNew string) []string {
	repo, err := git.PlainOpen(repodir)
	if err != nil {
		panic(err)
	}

	commit1, err := repo.CommitObject(plumbing.NewHash(commitOld))
	if err != nil {
		panic(err)
	}

	commit2, err := repo.CommitObject(plumbing.NewHash(commitNew))
	if err != nil {
		panic(err)
	}

	tree1, err := commit1.Tree()
	if err != nil {
		panic(err)
	}

	tree2, err := commit2.Tree()
	if err != nil {
		panic(err)
	}

	_, _, deletedFiles := compareFiles(repo, tree1, tree2)
	return deletedFiles
}

func compareFiles(repo *git.Repository, tree1, tree2 *object.Tree) ([]string, []string, []string) {
	files1 := getFilesFromTree(repo, tree1)
	files2 := getFilesFromTree(repo, tree2)

	modifiedFiles := []string{}
	appendedFiles := []string{}
	deletedFiles := []string{}
	// Check for deleted files

	for file := range files1 {
		if _, ok := files2[file]; !ok {
			deletedFiles = append(deletedFiles, file)
		}
	}

	// Check for appended and modified files
	for file, content2 := range files2 {
		if content1, ok := files1[file]; ok {
			if len(content1) != len(content2) && content1 != content2 {
				modifiedFiles = append(modifiedFiles, file)
			}
		} else {
			appendedFiles = append(appendedFiles, file)
		}
	}
	return modifiedFiles, appendedFiles, deletedFiles
}

func getFilesFromTree(repo *git.Repository, tree *object.Tree) map[string]string {
	files := make(map[string]string)
	walkTree(repo, tree, "", files)
	return files
}

func walkTree(repo *git.Repository, tree *object.Tree, path string, files map[string]string) {
	for _, entry := range tree.Entries {
		if entry.Mode == filemode.Regular {
			filePath := filepath.Join(path, entry.Name)
			fileContent, err := getFileContent(repo, entry.Hash)
			if err != nil {
				panic(err)
			}
			files[filePath] = fileContent
		} else if entry.Mode == filemode.Dir {
			subTree, err := object.GetTree(repo.Storer, entry.Hash)
			if err != nil {
				panic(err)
			}
			walkTree(repo, subTree, filepath.Join(path, entry.Name), files)
		}
	}
}

func getFileContent(repo *git.Repository, hash plumbing.Hash) (string, error) {
	blob, err := repo.BlobObject(hash)
	if err != nil {
		return "", err
	}
	reader, err := blob.Reader()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
