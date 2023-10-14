package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/gookit/color"
)

var watcher *fsnotify.Watcher

type Syncer struct {
	HostPath      string
	ContainerPath string
	UserName      string
	GroupName     string
	ContainerName string
}

func main() {
	syncer := Syncer{
		HostPath:      "",
		ContainerPath: "",
		UserName:      "",
		GroupName:     "",
		ContainerName: "",
	}
	flag.StringVar(&syncer.HostPath, "host-path", "", "Host path")
	flag.StringVar(&syncer.ContainerPath, "container-path", "", "Container path")
	flag.StringVar(&syncer.UserName, "user", "www-data", "User( default www-data )")
	flag.StringVar(&syncer.GroupName, "group", "www-data", "Group( default www-data )")
	flag.StringVar(&syncer.ContainerName, "container-name", "", "Container name")
	flag.Parse()
	color.Cyan.Println("Watching... ")
	checkDocker()
	color.Cyan.Print("Host path: ", syncer.HostPath)
	watchDir(syncer)
}

func watchDir(syncer Syncer) {
	// Create new watcher.
	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()

	// starting at the root of the project, walk each file/directory searching for
	// directories
	if err := filepath.Walk(syncer.HostPath, dirWalker); err != nil {
		fmt.Println("ERROR", err)
	}

	// Start listening for events.
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				eventHandler(event, syncer)
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
                color.Red.Print("error:", err)
			}
		}
	}()

	// Block main goroutine forever.
	<-make(chan struct{})
}

// watchDir gets run as a walk func, searching for directories to add watchers to
func dirWalker(path string, fi os.FileInfo, err error) error {

	// since fsnotify can watch all the files in a directory, watchers only need
	// to be added to each nested directory
	if fi == nil {
		return nil
	}
	if fi.Mode().IsDir() {
		return watcher.Add(path)
	}

	return nil
}

func eventHandler(event fsnotify.Event, syncer Syncer) {
	if strings.HasSuffix(event.Name, "~") {
        color.Cyan.Println("Skipping: ", event.Name)
		return
	}
	if event.Has(fsnotify.Write) {
		handleWrite(event, syncer)
	}
	if event.Has(fsnotify.Create) {
		handleCreate(event, syncer)
	}
	if event.Has(fsnotify.Remove) {
		handleRemove(event, syncer)
	}
	if event.Has(fsnotify.Rename) {
		handleRename(event, syncer)
	}
}

func handleWrite(event fsnotify.Event, syncer Syncer) {
	copyFromHostToContainer(event, syncer)
	applyPermissionsToFile(event, syncer)
}

func handleCreate(event fsnotify.Event, syncer Syncer) {
	copyFromHostToContainer(event, syncer)
	applyPermissionsToFile(event, syncer)
}

func handleRemove(event fsnotify.Event, syncer Syncer) {
	removeFromContainer(event, syncer)
}

func handleRename(event fsnotify.Event, syncer Syncer) {
	removeFromContainer(event, syncer)
}

func copyFromHostToContainer(event fsnotify.Event, syncer Syncer) {
	relativePath, err := getRelativePath(event, syncer)
	if err != nil {
		return
	}
	_, err = exec.Command("docker", "cp", event.Name, fmt.Sprintf("%s:%s", syncer.ContainerName, syncer.ContainerPath)).Output()
	if err != nil {
		color.Red.Println(err.Error(), "can't copy to container")
		return
	}
	message := fmt.Sprintf("Copied: %s to %s:%s", relativePath, syncer.ContainerName, syncer.ContainerPath+"/"+relativePath)
	color.Green.Println(message)
}

func removeFromContainer(event fsnotify.Event, syncer Syncer) {
	relativePath, err := getRelativePath(event, syncer)
	if err != nil {
		return
	}
	_, err = exec.Command("docker", "exec", syncer.ContainerName, "rm", syncer.ContainerPath+"/"+relativePath).Output()
	if err != nil {
		color.Red.Println(err.Error(), "cant remove from container")
		return
	}
	message := fmt.Sprintf("Removed: %s/%s", syncer.ContainerPath, relativePath)
	color.Red.Println(message)
}

func applyPermissionsToFile(event fsnotify.Event, s Syncer) {
	relativePath, err := getRelativePath(event, s)
	if err != nil {
		return
	}
	userGroup := fmt.Sprintf("%s:%s", s.UserName, s.GroupName)
	containerAbsolutePath := fmt.Sprintf("%s/%s", s.ContainerPath, relativePath)
	_, err = exec.Command("docker", "exec", s.ContainerName, "chown", userGroup, containerAbsolutePath).Output()
	if err != nil {
		color.Red.Println(err.Error(), "can't remove from container")
		return
	}
}

func checkDocker() {
	_, err := exec.Command("docker", "-v").Output()
	if err != nil {
		message := color.Red.Sprint("Docker command not found. Make sure docker is installed and running.")
		log.Fatal(message)
	}
}

func getRelativePath(event fsnotify.Event, syncer Syncer) (string, error) {
	relativePath, err := filepath.Rel(syncer.HostPath, event.Name)
	if err != nil {
		return "", err
	}
	return relativePath, nil
}
