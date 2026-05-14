#ifndef VZ_DARWIN_H
#define VZ_DARWIN_H

#include <stdint.h>

typedef struct {
    void *handle;
    char *error;
} vz_result;

// VM configuration
vz_result vz_create_vm_config(int cpus, uint64_t memory_bytes);

// VM lifecycle
char *vz_start_vm(void *vm_handle);
char *vz_stop_vm(void *vm_handle);

// Virtio-fs
vz_result vz_create_shared_directory(const char *path, const char *tag, int read_only);

// Network
vz_result vz_create_file_handle_network(int socket_fd);

#endif /* VZ_DARWIN_H */
