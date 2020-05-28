package main

import (
	"fmt"
	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox/files"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func FileExists(name string) bool {
	if fi, err := os.Stat(name); err == nil {
		return fi.Mode().IsRegular()
	}
	return false
}

func DirExists(name string) bool {
	if fi, err := os.Stat(name); err == nil {
		return fi.Mode().IsDir()
	}
	return false
}

// Sends a get_metadata request for a given path and returns the response
func getFileMetadata(c files.Client, path string) (files.IsMetadata, error) {
	arg := files.NewGetMetadataArg(path)

	res, err := c.GetMetadata(arg)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func getRemoteListing(c files.Client, path string) (entries []files.IsMetadata, path_exists bool) {
	// return vals
	entries = make([]files.IsMetadata, 0)
	path_exists = false

	arg := files.NewListFolderArg(path)
	res, err := c.ListFolder(arg)
	if err != nil {
		xType := fmt.Sprintf("%T", err)
		fmt.Println("ERROR TYOE IS: ", xType)
		switch e := err.(type) {
		case files.ListFolderAPIError:
			if e.EndpointError.Path.Tag == files.LookupErrorNotFolder {
				var metaRes files.IsMetadata
				metaRes, err = getFileMetadata(c, path)
				entries = []files.IsMetadata{metaRes}
			}
		case dropbox.APIError:
			fmt.Println("DISNAE EXIST!")
		}

	} else {
		entries = res.Entries
		path_exists = true

		for res.HasMore {
			arg := files.NewListFolderContinueArg(res.Cursor)

			res, err = c.ListFolderContinue(arg)

			if err == nil {
				entries = append(entries, res.Entries...)
			}

		}
	}
	return
}

func createRemoteDir(c files.Client, dir_name string) (bool, error) {

	arg := files.NewCreateFolderArg(dir_name)
	if _, err := c.CreateFolderV2(arg); err != nil {
		return false, err
	}
	return true, nil
}

func main() {
	fmt.Println("Syncb0ard!")

	args := os.Args
	if len(args) != 2 { // one is prog name, then one arg
		fmt.Fprintf(os.Stderr, "error: Need a directory name to sync\n")
		os.Exit(1)
	}

	dirName := args[1]

	if !DirExists(dirName) {
		fmt.Fprintf(os.Stderr, "error: Arg needs to be an existing directory name to sync\n")
		os.Exit(1)
	}

	fmt.Printf("Syncing %s\n", dirName)

	key, err := ioutil.ReadFile("key.txt")
	if err != nil {
		log.Fatal(err)
	}
	token := strings.TrimSuffix(string(key), "\n")
	config := dropbox.Config{
		Token:    token,
		LogLevel: dropbox.LogInfo,
	}

	dbx := files.New(config)

	remote_path := "/" + filepath.Base(dirName)
	entries, path_exists := getRemoteListing(dbx, remote_path)
	fmt.Printf("PATH EXIXTS? %s\n", path_exists)
	if !path_exists {
		createRemoteDir(dbx, remote_path)
	}

	for i, s := range entries {
		fmt.Println(i, s, "\n")
	}

}
