#ifndef VZ_DARWIN_H
#define VZ_DARWIN_H

#include <stdint.h>

typedef struct {
    void *handle;
    char *error;
} vz_result;

// --- Linux VM (relay) ---

// Creates and starts a Linux VM with the given kernel, initrd, and cmdline.
// Attaches virtio-fs shared directory and optionally a file-handle network
// device and a NAT network device.
// Returns a VM handle on success.
vz_result vz_create_linux_vm(
    int cpus,
    uint64_t memory_bytes,
    const char *kernel_path,
    const char *initrd_path,
    const char *cmdline,
    const char *shared_dir_path,
    const char *shared_dir_tag,
    int file_handle_socket_fd,  // -1 to skip
    int attach_nat              // non-zero to attach NAT device
);

// --- macOS VM (build) ---

// Creates and starts a macOS VM from a restored disk image.
// Requires hardware model + machine identifier + auxiliary storage
// (all produced during IPSW restore).
vz_result vz_create_macos_vm(
    int cpus,
    uint64_t memory_bytes,
    const char *disk_image_path,
    const char *aux_storage_path,
    const char *hardware_model_path,
    const char *machine_id_path,
    const char *shared_dir_path,
    const char *shared_dir_tag,
    int file_handle_socket_fd
);

// --- VM lifecycle ---

char *vz_start_vm(void *vm_handle);
char *vz_stop_vm(void *vm_handle);
int vz_vm_state(void *vm_handle);

// VM states (mirrors VZVirtualMachine.State)
#define VZ_STATE_STOPPED  0
#define VZ_STATE_RUNNING  1
#define VZ_STATE_PAUSED   2
#define VZ_STATE_ERROR    3
#define VZ_STATE_STARTING 4
#define VZ_STATE_STOPPING 5

// --- IPSW Restore ---

// Fetches the latest supported IPSW restore image URL from Apple.
// Returns the URL string (caller must free).
vz_result vz_latest_supported_ipsw(void);

// Restores a macOS IPSW to a disk image + platform state files.
// Creates: disk_path, aux_storage_path, hardware_model_path, machine_id_path.
// Progress is reported to stderr. Blocks until complete.
typedef void (*vz_progress_callback)(double fraction);
char *vz_restore_ipsw(
    const char *ipsw_path,
    const char *disk_path,
    uint64_t disk_size_bytes,
    const char *aux_storage_path,
    const char *hardware_model_path,
    const char *machine_id_path,
    vz_progress_callback progress_cb
);

#endif /* VZ_DARWIN_H */
