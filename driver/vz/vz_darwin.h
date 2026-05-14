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

#endif /* VZ_DARWIN_H */
