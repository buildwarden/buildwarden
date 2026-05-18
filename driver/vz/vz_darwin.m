#import <Virtualization/Virtualization.h>
#import <Foundation/Foundation.h>
#include "vz_darwin.h"
#include <stdlib.h>
#include <fcntl.h>
#include <unistd.h>

static char *copy_error(NSError *error) {
    if (error == nil) return NULL;
    NSMutableString *msg = [NSMutableString stringWithFormat:@"%@ (domain=%@ code=%ld)",
        [error localizedDescription], error.domain, (long)error.code];
    if (error.userInfo[NSUnderlyingErrorKey]) {
        NSError *underlying = error.userInfo[NSUnderlyingErrorKey];
        [msg appendFormat:@" [underlying: %@ domain=%@ code=%ld]",
            underlying.localizedDescription, underlying.domain, (long)underlying.code];
    }
    return strdup([msg UTF8String]);
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

    // Create VM on a dedicated serial queue (avoids main queue requirement)
    dispatch_queue_t vmQueue =
        dispatch_queue_create("com.buildwarden.vm.relay", DISPATCH_QUEUE_SERIAL);
    VZVirtualMachine *vm =
        [[VZVirtualMachine alloc] initWithConfiguration:config queue:vmQueue];

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

    // Create VM on a dedicated serial queue
    dispatch_queue_t vmQueue =
        dispatch_queue_create("com.buildwarden.vm.build", DISPATCH_QUEUE_SERIAL);
    VZVirtualMachine *vm =
        [[VZVirtualMachine alloc] initWithConfiguration:config queue:vmQueue];

    result.handle = (__bridge_retained void *)vm;
    return result;
}

// --- VM lifecycle ---

char *vz_start_vm(void *vm_handle) {
    VZVirtualMachine *vm = (__bridge VZVirtualMachine *)vm_handle;

    __block NSError *startError = nil;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);

    dispatch_async(vm.queue, ^{
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

    dispatch_async(vm.queue, ^{
        if (![vm canStop]) {
            stopError = [NSError errorWithDomain:@"VZWarden" code:1
                userInfo:@{NSLocalizedDescriptionKey:
                    @"VM cannot be stopped in current state"}];
            dispatch_semaphore_signal(sem);
            return;
        }
        [vm stopWithCompletionHandler:^(NSError *error) {
            stopError = error;
            dispatch_semaphore_signal(sem);
        }];
    });

    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
    return copy_error(stopError);
}

int vz_vm_state(void *vm_handle) {
    VZVirtualMachine *vm = (__bridge VZVirtualMachine *)vm_handle;
    return (int)vm.state;
}

// --- IPSW Restore ---

vz_result vz_latest_supported_ipsw(void) {
    vz_result result = {NULL, NULL};

    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    __block NSURL *ipswURL = nil;
    __block NSError *fetchError = nil;

    [VZMacOSRestoreImage fetchLatestSupportedWithCompletionHandler:
        ^(VZMacOSRestoreImage *restoreImage, NSError *error) {
            if (error != nil) {
                fetchError = error;
            } else {
                ipswURL = restoreImage.URL;
            }
            dispatch_semaphore_signal(sem);
        }];

    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);

    if (fetchError != nil) {
        result.error = copy_error(fetchError);
        return result;
    }

    const char *urlStr = [[ipswURL absoluteString] UTF8String];
    result.handle = (void *)strdup(urlStr);
    return result;
}

char *vz_restore_ipsw(
    const char *ipsw_path,
    const char *disk_path,
    uint64_t disk_size_bytes,
    const char *aux_storage_path,
    const char *hardware_model_path,
    const char *machine_id_path,
    vz_progress_callback progress_cb
) {
    NSString *nsIpsw = [NSString stringWithUTF8String:ipsw_path];
    NSString *nsDisk = [NSString stringWithUTF8String:disk_path];
    NSString *nsAux = [NSString stringWithUTF8String:aux_storage_path];
    NSString *nsHWModel = [NSString stringWithUTF8String:hardware_model_path];
    NSString *nsMachineId = [NSString stringWithUTF8String:machine_id_path];

    // Load the IPSW restore image
    dispatch_semaphore_t loadSem = dispatch_semaphore_create(0);
    __block VZMacOSRestoreImage *restoreImage = nil;
    __block NSError *loadError = nil;

    [VZMacOSRestoreImage loadFileURL:[NSURL fileURLWithPath:nsIpsw]
                   completionHandler:^(VZMacOSRestoreImage *image, NSError *error) {
        restoreImage = image;
        loadError = error;
        dispatch_semaphore_signal(loadSem);
    }];

    dispatch_semaphore_wait(loadSem, DISPATCH_TIME_FOREVER);
    if (loadError != nil) {
        return copy_error(loadError);
    }

    // Get the hardware configuration requirements
    VZMacOSConfigurationRequirements *requirements =
        restoreImage.mostFeaturefulSupportedConfiguration;
    if (requirements == nil) {
        return strdup("no supported configuration found in IPSW");
    }

    VZMacHardwareModel *hardwareModel = requirements.hardwareModel;

    // Save hardware model
    NSData *hwModelData = hardwareModel.dataRepresentation;
    if (![hwModelData writeToFile:nsHWModel atomically:YES]) {
        return strdup("failed to save hardware model");
    }

    // Generate and save machine identifier
    VZMacMachineIdentifier *machineId = [[VZMacMachineIdentifier alloc] init];
    NSData *machineIdData = machineId.dataRepresentation;
    if (![machineIdData writeToFile:nsMachineId atomically:YES]) {
        return strdup("failed to save machine identifier");
    }

    // Create auxiliary storage
    NSError *auxError = nil;
    VZMacAuxiliaryStorage *auxStorage =
        [[VZMacAuxiliaryStorage alloc]
            initCreatingStorageAtURL:[NSURL fileURLWithPath:nsAux]
                      hardwareModel:hardwareModel
                            options:VZMacAuxiliaryStorageInitializationOptionAllowOverwrite
                              error:&auxError];
    if (auxStorage == nil) {
        return copy_error(auxError);
    }

    // Create the disk image
    int fd = open([nsDisk UTF8String], O_RDWR | O_CREAT | O_TRUNC, 0644);
    if (fd < 0) {
        return strdup("failed to create disk image file");
    }
    if (ftruncate(fd, disk_size_bytes) != 0) {
        close(fd);
        return strdup("failed to set disk image size");
    }
    close(fd);

    // Configure the VM for installation
    VZVirtualMachineConfiguration *config =
        [[VZVirtualMachineConfiguration alloc] init];
    config.CPUCount = requirements.minimumSupportedCPUCount;
    config.memorySize = requirements.minimumSupportedMemorySize;

    // Platform
    VZMacPlatformConfiguration *platform =
        [[VZMacPlatformConfiguration alloc] init];
    platform.hardwareModel = hardwareModel;
    platform.machineIdentifier = machineId;
    platform.auxiliaryStorage = auxStorage;
    config.platform = platform;

    // Boot loader
    VZMacOSBootLoader *bootLoader = [[VZMacOSBootLoader alloc] init];
    config.bootLoader = bootLoader;

    // Disk
    NSError *diskError = nil;
    VZDiskImageStorageDeviceAttachment *diskAttachment =
        [[VZDiskImageStorageDeviceAttachment alloc]
            initWithURL:[NSURL fileURLWithPath:nsDisk]
               readOnly:NO
                  error:&diskError];
    if (diskAttachment == nil) {
        return copy_error(diskError);
    }
    VZVirtioBlockDeviceConfiguration *blockDevice =
        [[VZVirtioBlockDeviceConfiguration alloc]
            initWithAttachment:diskAttachment];
    config.storageDevices = @[blockDevice];

    // Network (NAT for installation — needs internet to activate)
    VZNATNetworkDeviceAttachment *natAttachment =
        [[VZNATNetworkDeviceAttachment alloc] init];
    VZVirtioNetworkDeviceConfiguration *netDevice =
        [[VZVirtioNetworkDeviceConfiguration alloc] init];
    netDevice.attachment = natAttachment;
    config.networkDevices = @[netDevice];

    // Validate
    NSError *valError = nil;
    if (![config validateWithError:&valError]) {
        return copy_error(valError);
    }

    // Create VM for installation.
    // VZMacOSInstaller requires the run loop to be serviced for internal XPC.
    // We use a dedicated serial queue and CFRunLoop to ensure XPC callbacks
    // are processed throughout the long-running installation.
    dispatch_queue_t installQueue =
        dispatch_queue_create("com.buildwarden.vm.install", DISPATCH_QUEUE_SERIAL);

    __block VZVirtualMachine *vm =
        [[VZVirtualMachine alloc] initWithConfiguration:config queue:installQueue];

    __block NSError *installError = nil;
    __block BOOL installDone = NO;

    dispatch_async(installQueue, ^{
        VZMacOSInstaller *installer =
            [[VZMacOSInstaller alloc] initWithVirtualMachine:vm
                                           restoreImageURL:[NSURL fileURLWithPath:nsIpsw]];

        // Log progress periodically from a background thread
        dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
            while (!installer.progress.finished && !installer.progress.cancelled) {
                fprintf(stderr, "\rRestoring macOS: %.0f%%",
                        installer.progress.fractionCompleted * 100.0);
                [NSThread sleepForTimeInterval:2.0];
            }
            fprintf(stderr, "\rRestoring macOS: done.     \n");
        });

        [installer installWithCompletionHandler:^(NSError *error) {
            installError = error;
            installDone = YES;
            CFRunLoopStop(CFRunLoopGetMain());
        }];
    });

    // Run the main run loop to service XPC/VZ internal callbacks.
    while (!installDone) {
        CFRunLoopRunInMode(kCFRunLoopDefaultMode, 1.0, true);
    }

    if (installError != nil) {
        return copy_error(installError);
    }

    // Stop the installation VM so the disk image is released.
    dispatch_semaphore_t stopSem = dispatch_semaphore_create(0);
    dispatch_async(installQueue, ^{
        if ([vm canStop]) {
            [vm stopWithCompletionHandler:^(NSError *error) {
                dispatch_semaphore_signal(stopSem);
            }];
        } else {
            dispatch_semaphore_signal(stopSem);
        }
    });
    dispatch_semaphore_wait(stopSem, dispatch_time(DISPATCH_TIME_NOW, 10 * NSEC_PER_SEC));

    return NULL;
}
