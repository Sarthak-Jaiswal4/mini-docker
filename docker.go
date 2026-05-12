package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

type Container struct {
        ID      string
        PID     int
        Status  string
        Command []string
        Created string
}

func main() {
        fmt.Println("Args:", os.Args)

        if len(os.Args) < 2 {
                panic("missing command")
        }

        switch os.Args[1] {

        case "run":
                if len(os.Args) < 3 {  
                        fmt.Println("usage: mini-docker run [-d] <command>")
                        return
                }
                run()
        case "child":
                child()
        case "ps":
                listContainers()
        case "stop":
                stop()
        case "rm":
                Cleanup()
        case "exec":
                execute()
        default:
                panic("unknown command")
        }
}

func StoreMetadata(id string,location string,pid int){

        Command := os.Args[2:]
        if os.Args[2] == "-d" {
                Command = os.Args[3:]  
        }

        data := Container{
                ID:id,
                PID:pid,
                Status: "Running",
                Command:Command,    
                Created: time.Now().String(),
        }

        jsonData, err := json.MarshalIndent(data, "", "  ")
        if err != nil {
                fmt.Println("Error",err)
        }

        err = os.WriteFile(location+"/config.json", jsonData, 0644)
        if err != nil {
                fmt.Println("Error",err)
        }

        fmt.Println("Metadata stored at:", location+"/config.json")
}

func CreateContainer(ID string) (string,error) {

        location := "container/"+ID

        err := os.MkdirAll(location+"/work", 0755)
        if err != nil {
                return "", err
        }

        err = os.MkdirAll(location+"/upper", 0755)
        if err != nil {
                return "", err
        }

        err = os.MkdirAll(location+"/merged", 0755)
        if err != nil {
                return "", err
        }

        opts := fmt.Sprintf(
                "lowerdir=overlay/lower,upperdir=%s,workdir=%s",
                location+"/upper",
                location+"/work",
        )
        
	err = syscall.Mount("overlay", location+"/merged", "overlay", 0, opts)
        if err != nil {
                return "", fmt.Errorf("overlay mount: %w", err)
        }
        
        err = os.MkdirAll(location+"/merged/.put_old", 0755)
        if err != nil {
                return "", err
        }

	if err != nil {
		return "", fmt.Errorf("overlay mount: %w", err)
	}

        fmt.Println("Overlay mounted at:", location+"/merged")

        return location, nil
}       

func cGroup(pid int,id string){
        location := "/sys/fs/cgroup/"+id

        err := os.MkdirAll(location,0755)
        if err!=nil{
                fmt.Println("Error in creating file",err)
        }

        err = os.WriteFile(location+"/memory.max",[]byte("104857600"),0644)
        if err != nil {
                fmt.Println("Error setting memory limit:", err)
                return
        }

        err = os.WriteFile(location+"/cgroup.procs",[]byte(strconv.Itoa(pid)+ "\n"),0644)
        if err != nil {
                fmt.Println("Error adding PID:", err)
                return
        }
}

func Cleanup(){
        id := os.Args[2]

        location := "container/"+id

        data, err := os.ReadFile(location + "/config.json")
        if err != nil {
                fmt.Println("Container not found:", err)
                return
        }

        var cfg Container
        json.Unmarshal(data, &cfg)

        if cfg.Status == "Running" {
                fmt.Println("Stop container first before removing")
                return
        }

        err = syscall.Unmount(location+"/merged",0)
        if err != nil {
                fmt.Println("Unmount error:", err)
        }

        err = os.RemoveAll(location)
        if err != nil {
                fmt.Println("Filesystem cleanup error:", err)
        }

        err = os.RemoveAll("/sys/fs/cgroup/"+id)
        if err != nil {
                fmt.Println("Cgroup cleanup error:", err)
        }
}

func stop() {

        if len(os.Args) < 3 {
                fmt.Println("missing container id")
                return
        }

        containerID := os.Args[2]

        configPath := "container/" + containerID + "/config.json"

        data, err := os.ReadFile(configPath)
        if err != nil {
                fmt.Println("ReadFile error:", err)
                return
        }

        var cfg Container

        err = json.Unmarshal(data, &cfg)
        if cfg.Status == "Stopped" {
                fmt.Println("Container already stopped")
                return
        }   

        if err != nil {
                fmt.Println("Unmarshal error:", err)
                return
        }

        process, err := os.FindProcess(cfg.PID)
        if err != nil {
                fmt.Println("FindProcess error:", err)
                return
        }

        err = process.Signal(syscall.SIGTERM)
        if err != nil {
                fmt.Println("SIGTERM error:", err)
        }

        time.Sleep(5 * time.Second)

        err = process.Signal(syscall.Signal(0))
        if err == nil {
                fmt.Println("Process still running, sending SIGKILL")
                process.Signal(syscall.SIGKILL)
        }

        cfg.Status = "Stopped"

        updated, err := json.MarshalIndent(cfg, "", "  ")
        if err != nil {
                fmt.Println("Marshal error:", err)
                return
        }

        err = os.WriteFile(configPath, updated, 0644)
        if err != nil {
                fmt.Println("WriteFile error:", err)
                return
        }

        fmt.Println("Container stopped:", containerID)
}

func execute(){
        runtime.LockOSThread()
        
        if len(os.Args) < 4 {
		fmt.Println("usage: mini-docker exec <container-id> <command>")
		return
	}

        containerid:=os.Args[2]

        fd,err := os.ReadFile("container/"+containerid+"/config.json")

        if err!=nil{
                fmt.Println("Error in reading file",err)
        }

        var cfg Container
        err =json.Unmarshal(fd,&cfg)

        if err != nil {
		fmt.Println("Unmarshal error:", err)
		return
	}

        if cfg.Status!="Running"{
                fmt.Println("Container not running")
                return
        }

        process, err := os.FindProcess(cfg.PID)
	if err != nil {
		fmt.Println("FindProcess error:", err)
		return
	}

	err = process.Signal(syscall.Signal(0))
	if err != nil {
		fmt.Println("Container process is dead:", err)
		return
	}

        namespaces := []struct {
		path string
		flag uintptr
		name string
	}{
		{
			path: fmt.Sprintf("/proc/%d/ns/mnt", cfg.PID),
			flag: syscall.CLONE_NEWNS,
			name: "mount",
		},
		{
			path: fmt.Sprintf("/proc/%d/ns/net", cfg.PID),
			flag: syscall.CLONE_NEWNET,
			name: "network",
		},
		{
			path: fmt.Sprintf("/proc/%d/ns/pid", cfg.PID),
			flag: syscall.CLONE_NEWPID,
			name: "PID",
		},
	}

        for _, ns := range namespaces {

		fd, err := os.Open(ns.path)
		if err != nil {
			fmt.Printf("open %s namespace error: %v\n", ns.name, err)
			return
		}
		_, _, errno := syscall.RawSyscall(
			syscall.SYS_SETNS,
			fd.Fd(),
			ns.flag,
			0,
		)
                fd.Close()

		if errno != 0 {
			fmt.Printf("setns %s error: %v\n", ns.name, errno)
			return
		}

		fmt.Printf("joined %s namespace ✅\n", ns.name)
	}

        cmd := exec.Command(
                os.Args[0],
                append(
                        []string{"exec-child"},
                        os.Args[3:]...,
                )...,
        )
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		fmt.Println("Exec error:", err)
	}
}

func execChild() {
        syscall.Unmount("/proc", 0)

        err := syscall.Mount(
                "proc",
                "/proc",
                "proc",
                0,
                "",
        )

        if err != nil {
                fmt.Println("proc mount error:", err)
                return
        }

        cmd := exec.Command(
                os.Args[2],
                os.Args[3:]...,
        )

        cmd.Stdin = os.Stdin
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr

        err = cmd.Run()
        if err != nil {
                fmt.Println("exec-child error:", err)
        }
}

func listContainers(){
        entries, err := os.ReadDir("container")
        if err != nil {
                fmt.Println("ReadDir error:", err)
                return
        }

        fmt.Printf(
                "%-15s %-10s %-12s %-20s\n",
                "CONTAINER ID",
                "PID",
                "STATUS",
                "COMMAND",
        )

        for _, entry := range entries {

                if !entry.IsDir() {
                        continue
                }

                configPath := "container/" + entry.Name() + "/config.json"

                data, err := os.ReadFile(configPath)
                if err != nil {
                        fmt.Println("ReadFile error:", err)
                        continue
                }

                var container Container

                err = json.Unmarshal(data, &container)
                if err != nil {
                        fmt.Println("Unmarshal error:", err)
                        continue
                }

                command := ""
                for _, c := range container.Command {
                        command += c + " "
                }

                fmt.Printf(
                        "%-15s %-10d %-12s %-20s\n",
                        container.ID,
                        container.PID,
                        container.Status,
                        command,
                )
        }
}

func updateStatus(location string,status string){

        data, err := os.ReadFile(location+"/config.json")

        if err!=nil{
                fmt.Println("Error in reading file",err)
                return
        }

        var cfg Container

        err = json.Unmarshal(data,&cfg)
        if err!=nil{
                fmt.Println("Error in unmarshal",err)
                return
        }

        cfg.Status=status
        if status == "Stopped" {  
                cfg.PID = 0
        }

        jsonData,_ := json.MarshalIndent(cfg,""," ")

        err = os.WriteFile(location+"/config.json", jsonData, 0644)
        if err!=nil{
                fmt.Println("Error in writing file",err)
                return
        }
}

func run() {
        fmt.Println("Running in parent")

        ID := strconv.FormatInt(time.Now().UnixNano(), 36)

        location,err := CreateContainer(ID)

        if err!=nil{
                fmt.Println("Error",err)
                return
        }

        var cmd *exec.Cmd
        var detached bool = false
        var nullFile, stdoutLog, stderrLog *os.File

        if os.Args[2]=="-d"{
                if len(os.Args) < 4 {
                        fmt.Println("usage: mini-docker run -d <command>")
                        return
                }

                detached=true

                cmd = exec.Command(os.Args[0], append([]string{"child",location+"/merged"}, os.Args[3:]...)...)

                cmd.SysProcAttr = &syscall.SysProcAttr{
                        Setsid:true,
                        Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
                }
        }else{
                cmd = exec.Command(os.Args[0], append([]string{"child",location+"/merged"}, os.Args[2:]...)...)

                cmd.SysProcAttr = &syscall.SysProcAttr{
                        Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
                }
        }

        if detached {
                nullFile, err = os.Open("/dev/null")
                if err != nil {
                        fmt.Println("Error opening /dev/null:", err)
                        return
                }

                os.MkdirAll("container/"+ID+"/logs", 0755)

                stdoutLog, err = os.Create("container/" + ID + "/logs/stdout.log")
                if err != nil {
                        fmt.Println("Error creating stdout log:", err)
                        return
                }

                stderrLog, err = os.Create("container/" + ID + "/logs/stderr.log")
                if err != nil {
                        fmt.Println("Error creating stderr log:", err)
                        return
                }

                cmd.Stdin  = nullFile 
                cmd.Stdout = stdoutLog  
                cmd.Stderr = stderrLog  
        } else {
                cmd.Stdin  = os.Stdin
                cmd.Stdout = os.Stdout
                cmd.Stderr = os.Stderr
        }

        err = cmd.Start()
        if err != nil {
                fmt.Println("Start error:", err)
                if detached {
                        nullFile.Close() 
                        stdoutLog.Close()
                        stderrLog.Close()
                }
                return
        }

        if detached {
                nullFile.Close()
                stdoutLog.Close()
                stderrLog.Close()
        }

        StoreMetadata(ID,location,cmd.Process.Pid)

        cGroup(cmd.Process.Pid,ID)

        if detached==true{
                fmt.Println("Container started:", ID)
                return
        }

        err = cmd.Wait()
        updateStatus(location,"Stopped")

        if err != nil {
                fmt.Println("Error:", err)
        }

        fmt.Println("parent function closed")
}

func child() {
        fmt.Println("Running in child (inside namespace)")

        newpath := os.Args[2]

        err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, "")
        if err != nil {
                fmt.Println("Private mount error:", err)
                return
        }

        // Bind mount rootfs onto itself
        err = syscall.Mount(newpath, newpath, "", syscall.MS_BIND, "")
        if err != nil {
                fmt.Println("Bind mount error:", err)
                return
        }

        // Pivot root
        err = syscall.PivotRoot(newpath, newpath+"/.put_old")
        if err != nil {
                fmt.Println("PivotRoot error:", err)
                return
        }

        // Change cwd to new root
        err = os.Chdir("/")
        if err != nil {
                fmt.Println("Chdir error:", err)
                return
        }

        // Unmount old root
        err = syscall.Unmount("/.put_old", syscall.MNT_DETACH)
        if err != nil {
                fmt.Println("Unmount error:", err)
                return
        }

        // Remove old root dir
        err = os.Remove("/.put_old")
        if err != nil {
                fmt.Println("Remove error:", err)
                return
        }

        // Mount proc
        err = syscall.Mount("proc", "/proc", "proc", 0, "")
        if err != nil {
                fmt.Println("Proc mount error:", err)
                return
        }

        cmd := exec.Command(os.Args[3], os.Args[4:]...)

        cmd.Stdin = os.Stdin
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr

        err = cmd.Run()
        if err != nil {
                fmt.Println("Command error:", err)
        }
}