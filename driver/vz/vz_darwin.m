#import <Virtualization/Virtualization.h>
#import <Foundation/Foundation.h>
#include "vz_darwin.h"
#include <stdlib.h>

static char *copy_error(NSError *error) {
    if (error == nil) return NULL;
    const char *desc = [[error localizedDescription] UTF8String];
    return strdup(desc);
}

// Helper: create a virtio-fs device configuration
static VZVirtioFileSystemDeviceConfiguration *
make_virtiofs(const char *path, const char *tag) {
    NSString *nsPath = [NSString stringWithUTF8String:path];
    NSString *nsTag = [NSString stringWithUTF8String:tag];

    VZSharedDirectory *shared =
        [[VZSharedDirectory alloc] initWithURL:[NSURL fileURLWithPath:nsPath]
                                      readOnly:NO];
    VZSingleDirectoryShare *share =
        [[VZSingleDirectoryShare alloc] initWithDirectory:shared];

    VZVirtioFileSystemDeviceConfiguration *fs =
        [[VZVirtioFileSystemDeviceConfiguration alloc] initWithTag:nsTag];
    fs.share = share;
    return fs;
}

// Helper: create a file-handle network device from a socket FD
static VZVirtioNetworkDeviceConfiguration *
make_file_handle_net(int socket_fd) {
    NSFileHandle *fh =
        [[NSFileHandle alloc] initWithFileDescriptor:socket_fd
                                      closeOnDealloc:NO];
    VZFileHandleNetworkDeviceAttachment *attachment =
        [[VZFileHandleNetworkDeviceAttachment alloc] initWithFileHandle:fh];

    VZVirtioNetworkDeviceConfiguration *net =
        [[VZVirtioNetworkDeviceConfiguration alloc] init];
    net.attachment = attachment;
    return net;
}

// Helper: create a NAT network device (internet access via host)
static VZVirtioNetworkDeviceConfiguration *
make_nat_net(void) {
    VZNATNetworkDeviceAttachment *attachment =
        [[VZNATNetworkDeviceAttachment alloc] init];

    VZVirtioNetworkDeviceConfiguration *net =
        [[VZVirtioNetworkDeviceConfiguration alloc] init];
    net.attachment = attachment;
    return net;
}

// --- Linux VM (relay) ---

vz_result vz_create_linux_vm(
    int cpus,
    uint64_t memory_bytes,
    const char *kernel_path,
    const char *initrd_path,
    const char *cmdline,
    const char *shared_dir_path,
    const char *shared_dir_tag,
    int file_handle_socket_fd,
    int attach_nat
) {
    vz_result result = {NULL, NULL};

    VZVirtualMachineConfiguration *config =
        [[VZVirtualMachineConfiguration alloc] init];
    config.CPUCount = cpus;
    config.memorySize = memory_bytes;

    // Linux boot loader
    NSString *nsKernel = [NSString stringWithUTF8String:kernel_path];
    VZLinuxBootLoader *bootLoader =
        [[VZLinuxBootLoader alloc]
            initWithKernelURL:[NSURL fileURLWithPath:nsKernel]];

    if (initrd_path != NULL) {
        NSString *nsInitrd = [NSString stringWithUTF8String:initrd_path];
        bootLoader.initialRamdiskURL = [NSURL fileURLWithPath:nsInitrd];
    }
    if (cmdline != NULL) {
        bootLoader.commandLine = [NSString stringWithUTF8String:cmdline];
    }
    config.bootLoader = bootLoader;

    // Virtio-fs shared directory
    if (shared_dir_path != NULL && shared_dir_tag != NULL) {
        VZVirtioFileSystemDeviceConfiguration *fs =
            make_virtiofs(shared_dir_path, shared_dir_tag);
        config.directorySharingDevices = @[fs];
    }

    // Network devices
    NSMutableArray *netDevices = [[NSMutableArray alloc] init];

    if (file_handle_socket_fd >= 0) {
        [netDevices addObject:make_file_handle_net(file_handle_socket_fd)];
    }
    if (attach_nat) {
        [netDevices addObject:make_nat_net()];
    }
    if (netDevices.count > 0) {
        config.networkDevices = netDevices;
    }

    // Serial console (for debugging — maps to stdout)
    VZVirtioConsoleDeviceSerialPortConfiguration *serial =
        [[VZVirtioConsoleDeviceSerialPortConfiguration alloc] init];
    NSFileHandle *stdoutHandle = [NSFileHandle fileHandleWithStandardOutput];
    NSFileHandle *stdinHandle = [NSFileHandle fileHandleWithStandardInput];
    VZFileHandleSerialPortAttachment *serialAttachment =
        [[VZFileHandleSerialPortAttachment alloc]
            initWithFileHandleForReading:stdinHandle
                   fileHandleForWriting:stdoutHandle];
    serial.attachment = serialAttachment;
    config.serialPorts = @[serial];

    // Entropy device (improves boot randomness)
    VZVirtioEntropyDeviceConfiguration *entropy =
        [[VZVirtioEntropyDeviceConfiguration alloc] init];
    config.entropyDevices = @[entropy];

    // Validate
    NSError *error = nil;
    if (![config validateWithError:&error]) {
        result.error = copy_error(error);
        return result;
    }

    // Create VM
    VZVirtualMachine *vm =
        [[VZVirtualMachine alloc] initWithConfiguration:config];

    result.handle = (__bridge_retained void *)vm;
    return result;
}

// --- macOS VM (build) ---

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
) {
    vz_result result = {NULL, NULL};

    VZVirtualMachineConfiguration *config =
        [[VZVirtualMachineConfiguration alloc] init];
    config.CPUCount = cpus;
    config.memorySize = memory_bytes;

    // macOS platform configuration
    NSString *nsHWModel = [NSString stringWithUTF8String:hardware_model_path];
    NSData *hwModelData = [NSData dataWithContentsOfFile:nsHWModel];
    if (hwModelData == nil) {
        result.error = strdup("failed to read hardware model file");
        return result;
    }

    VZMacHardwareModel *hwModel =
        [[VZMacHardwareModel alloc] initWithDataRepresentation:hwModelData];
    if (hwModel == nil) {
        result.error = strdup("invalid hardware model data");
        return result;
    }

    NSString *nsMachineId = [NSString stringWithUTF8String:machine_id_path];
    NSData *machineIdData = [NSData dataWithContentsOfFile:nsMachineId];
    if (machineIdData == nil) {
        result.error = strdup("failed to read machine identifier file");
        return result;
    }

    VZMacMachineIdentifier *machineId =
        [[VZMacMachineIdentifier alloc]
            initWithDataRepresentation:machineIdData];
    if (machineId == nil) {
        result.error = strdup("invalid machine identifier data");
        return result;
    }

    NSString *nsAux = [NSString stringWithUTF8String:aux_storage_path];
    NSError *error = nil;
    VZMacAuxiliaryStorage *auxStorage =
        [[VZMacAuxiliaryStorage alloc]
            initWithURL:[NSURL fileURLWithPath:nsAux]];

    VZMacPlatformConfiguration *platform =
        [[VZMacPlatformConfiguration alloc] init];
    platform.hardwareModel = hwModel;
    platform.machineIdentifier = machineId;
    platform.auxiliaryStorage = auxStorage;
    config.platform = platform;

    // macOS boot loader
    VZMacOSBootLoader *bootLoader = [[VZMacOSBootLoader alloc] init];
    config.bootLoader = bootLoader;

    // Disk image
    NSString *nsDisk = [NSString stringWithUTF8String:disk_image_path];
    VZDiskImageStorageDeviceAttachment *diskAttachment =
        [[VZDiskImageStorageDeviceAttachment alloc]
            initWithURL:[NSURL fileURLWithPath:nsDisk]
               readOnly:NO
                  error:&error];
    if (diskAttachment == nil) {
        result.error = copy_error(error);
        return result;
    }

    VZVirtioBlockDeviceConfiguration *disk =
        [[VZVirtioBlockDeviceConfiguration alloc]
            initWithAttachment:diskAttachment];
    config.storageDevices = @[disk];

    // Virtio-fs shared directory
    if (shared_dir_path != NULL && shared_dir_tag != NULL) {
        VZVirtioFileSystemDeviceConfiguration *fs =
            make_virtiofs(shared_dir_path, shared_dir_tag);
        config.directorySharingDevices = @[fs];
    }

    // Network: file-handle only (isolated — relay is sole gateway)
    if (file_handle_socket_fd >= 0) {
        config.networkDevices =
            @[make_file_handle_net(file_handle_socket_fd)];
    }

    // Validate
    if (![config validateWithError:&error]) {
        result.error = copy_error(error);
        return result;
    }

    // Create VM
    VZVirtualMachine *vm =
        [[VZVirtualMachine alloc] initWithConfiguration:config];

    result.handle = (__bridge_retained void *)vm;
    return result;
}

// --- VM lifecycle ---

char *vz_start_vm(void *vm_handle) {
    VZVirtualMachine *vm = (__bridge VZVirtualMachine *)vm_handle;

    __block NSError *startError = nil;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);

    [vm startWithCompletionHandler:^(NSError *error) {
        startError = error;
        dispatch_semaphore_signal(sem);
    }];

    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
    return copy_error(startError);
}

char *vz_stop_vm(void *vm_handle) {
    VZVirtualMachine *vm = (__bridge VZVirtualMachine *)vm_handle;

    if (![vm canStop]) {
        return strdup("VM cannot be stopped in current state");
    }

    __block NSError *stopError = nil;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);

    [vm stopWithCompletionHandler:^(NSError *error) {
        stopError = error;
        dispatch_semaphore_signal(sem);
    }];

    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
    return copy_error(stopError);
}

int vz_vm_state(void *vm_handle) {
    VZVirtualMachine *vm = (__bridge VZVirtualMachine *)vm_handle;
    return (int)vm.state;
}
