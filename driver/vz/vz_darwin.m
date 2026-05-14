#import <Virtualization/Virtualization.h>
#import <Foundation/Foundation.h>
#include "vz_darwin.h"
#include <stdlib.h>

static char *copy_error(NSError *error) {
    if (error == nil) return NULL;
    const char *desc = [[error localizedDescription] UTF8String];
    return strdup(desc);
}

vz_result vz_create_vm_config(int cpus, uint64_t memory_bytes) {
    vz_result result = {NULL, NULL};

    VZVirtualMachineConfiguration *config =
        [[VZVirtualMachineConfiguration alloc] init];
    config.CPUCount = cpus;
    config.memorySize = memory_bytes;

    // Boot loader for macOS guests
    VZMacOSBootLoader *bootLoader = [[VZMacOSBootLoader alloc] init];
    config.bootLoader = bootLoader;

    // Validate configuration
    NSError *error = nil;
    if (![config validateWithError:&error]) {
        result.error = copy_error(error);
        return result;
    }

    // Create VM (not yet started)
    VZVirtualMachine *vm = [[VZVirtualMachine alloc]
        initWithConfiguration:config];

    result.handle = (__bridge_retained void *)vm;
    return result;
}

char *vz_start_vm(void *vm_handle) {
    VZVirtualMachine *vm = (__bridge VZVirtualMachine *)vm_handle;

    __block NSError *startError = nil;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);

    dispatch_async(dispatch_get_main_queue(), ^{
        [vm startWithCompletionHandler:^(NSError *error) {
            startError = error;
            dispatch_semaphore_signal(sem);
        }];
    });

    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
    return copy_error(startError);
}

char *vz_stop_vm(void *vm_handle) {
    VZVirtualMachine *vm = (__bridge VZVirtualMachine *)vm_handle;

    __block NSError *stopError = nil;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);

    dispatch_async(dispatch_get_main_queue(), ^{
        [vm stopWithCompletionHandler:^(NSError *error) {
            stopError = error;
            dispatch_semaphore_signal(sem);
        }];
    });

    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
    return copy_error(stopError);
}

vz_result vz_create_shared_directory(const char *path, const char *tag,
                                     int read_only) {
    vz_result result = {NULL, NULL};

    NSString *nsPath = [NSString stringWithUTF8String:path];
    NSString *nsTag = [NSString stringWithUTF8String:tag];

    VZSharedDirectory *shared =
        [[VZSharedDirectory alloc] initWithURL:[NSURL fileURLWithPath:nsPath]
                                      readOnly:(read_only != 0)];

    VZSingleDirectoryShare *share =
        [[VZSingleDirectoryShare alloc] initWithDirectory:shared];

    VZVirtioFileSystemDeviceConfiguration *fsDevice =
        [[VZVirtioFileSystemDeviceConfiguration alloc] initWithTag:nsTag];
    fsDevice.share = share;

    result.handle = (__bridge_retained void *)fsDevice;
    return result;
}

vz_result vz_create_file_handle_network(int socket_fd) {
    vz_result result = {NULL, NULL};

    NSFileHandle *fileHandle =
        [[NSFileHandle alloc] initWithFileDescriptor:socket_fd
                                      closeOnDealloc:NO];

    VZFileHandleNetworkDeviceAttachment *attachment =
        [[VZFileHandleNetworkDeviceAttachment alloc]
            initWithFileHandle:fileHandle];

    VZVirtioNetworkDeviceConfiguration *netDevice =
        [[VZVirtioNetworkDeviceConfiguration alloc] init];
    netDevice.attachment = attachment;

    result.handle = (__bridge_retained void *)netDevice;
    return result;
}
