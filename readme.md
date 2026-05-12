**Mini-Docker: A Go-Based Container Engine**
--------------------------------------------

This project is a custom-built container runtime implemented in **Go**. It leverages core Linux kernel features---**Namespaces**, **Control Groups (cgroups)**, and **Overlay Filesystems**---to provide process isolation and resource management, similar to how Docker operates under the hood.

* * * * *

**Key Features**
----------------

-   **Process Isolation:** Uses Linux Namespaces (`PID`, `Mount`, `Network`) to isolate the container from the host.

-   **Layered Filesystem:** Implements `OverlayFS` to create a merged view of lower (read-only) and upper (writeable) layers.

-   **Resource Constraints:** Utilizes `cgroups v2` to limit memory usage (defaulted to 100MB).

-   **Lifecycle Management:** Support for `run`, `stop`, `ps`, and `rm` commands.

-   **Container Execution:** Includes an `exec` command to enter a running container by joining its existing namespaces.

-   **Detached Mode:** Support for the `-d` flag to run containers in the background with persistent logging.

* * * * *

**Technical Workflow**
----------------------

### **1\. The Initialization Flow (Run)**

1.  **ID Generation:** A unique ID is generated using a base-36 timestamp.

2.  **Filesystem Setup:** `CreateContainer()` creates `work`, `upper`, and `merged` directories. It mounts an `overlay` filesystem.

3.  **Namespace Forking:** The parent process re-executes itself with `Cloneflags`. This creates a new process in isolated PID, Network, and Mount namespaces.

4.  **Metadata & Cgroups:** The parent saves container details to `config.json` and writes the child's PID to `/sys/fs/cgroup` to enforce memory limits.

### **2\. The Jailbreak (Child Process)**

1.  **PivotRoot:** Instead of a simple `chroot`, the code uses `PivotRoot` to move the entire root filesystem to the overlay `merged` directory and unmounts the old host root.

2.  **Proc Mounting:** A fresh `/proc` is mounted so the container has its own isolated process list.

3.  **Command Execution:** The target application (e.g., `/bin/sh`) is executed as PID 1 inside the container.

### **3\. The Entry Flow (Exec)**

1.  **Namespace Joining:** The `exec` command identifies the target container's PID from its metadata.

2.  **Setns:** It uses `syscall.RawSyscall` with `SYS_SETNS` to switch the current thread into the container's namespaces before running the new command.

* * * * *

**Project Structure**
---------------------

Plaintext

```
.
├── main.go               # The entire container engine logic
├── overlay/              # Base OS layers
│   └── lower/            # The read-only rootfs (e.g., Alpine or Ubuntu)
├── container/            # Active container data
│   └── <ID>/
│       ├── config.json   # Container state and metadata
│       ├── merged/       # The actual root seen by the container
│       ├── upper/        # Changes made inside the container
│       └── logs/         # Stdout/Stderr for detached containers

```

* * * * *

**Usage Guide**
---------------

### **Commands**

| **Command** | **Usage** | **Description** |
| --- | --- | --- |
| **run** | `sudo ./mini-docker run /bin/sh` | Start a container interactively. |
| **run -d** | `sudo ./mini-docker run -d top` | Start a container in the background. |
| **ps** | `./mini-docker ps` | List all containers and their status. |
| **exec** | `sudo ./mini-docker exec <id> /bin/ls` | Run a command in a running container. |
| **stop** | `sudo ./mini-docker stop <id>` | Signal the container process to terminate. |
| **rm** | `sudo ./mini-docker rm <id>` | Unmount filesystems and delete container data. |

### **Setup Requirements**

1.  **Kernel:** Must be running Linux (Namespaces and Cgroups are not available on macOS/Windows natively).

2.  **Permissions:** Most operations require `sudo` to manipulate namespaces and mounts.

3.  **Rootfs:** Ensure a valid Linux distribution exists in `overlay/lower`.

* * * * *

**Internal Architecture Flowchart**
-----------------------------------

Code snippet

```
graph TD
    A[User Command: run] --> B{Check Flags}
    B -- -d --> C[Set Sid & Redirect Logs to File]
    B -- interactive --> D[Connect Stdin/Out/Err to Host]
    C --> E[Fork Child with CLONE_NEW...]
    D --> E
    E --> F[Parent: Store Metadata & Set Cgroups]
    E --> G[Child: PivotRoot to Merged Overlay]
    G --> H[Child: Mount /proc]
    H --> I[Child: Exec Target Command]
    I --> J[Container Running]
    J --> K[User Command: ps/stop/exec]

```

* * * * *

**Implementation Details**
--------------------------

-   **Metadata:** Stored in `config.json` using the `Container` struct.

-   **Memory Limit:** Hardcoded to **100MB** in the `cGroup` function via `memory.max`.

-   **Cleanup:** The `rm` command handles unmounting the overlay filesystem before deleting directories to prevent host resource leaks.