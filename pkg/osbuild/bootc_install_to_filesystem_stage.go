package osbuild

import (
	"fmt"

	"slices"
	"strings"

	"github.com/osbuild/image-builder/pkg/platform"
)

type BootcInstallToFilesystemOptions struct {
	// options for --root-ssh-authorized-keys
	RootSSHAuthorizedKeys []string `json:"root-ssh-authorized-keys,omitempty"`
	// options for --karg
	Kargs []string `json:"kernel-args,omitempty"`

	// option for --target-imgref
	TargetImgref string `json:"target-imgref"`

	Bootloader string `json:"bootloader,omitempty"`

	ComposeFS *bool `json:"composefs,omitempty"`

	// option for --initialize-var-mounts
	InitializeVarMounts bool `json:"initialize-var-mounts,omitempty"`

	// option for --defer-var
	DeferVar bool `json:"defer-var,omitempty"`
}

func (BootcInstallToFilesystemOptions) isStageOptions() {}

// BootcInstallVarMounts says what the bootc that runs the installation can do
// with the filesystems mounted at /var and below.
type BootcInstallVarMounts struct {
	// Initialize them from the image (--initialize-var-mounts). Without
	// it, they are not mounted for the installation.
	Initialize bool
	// Also skip seeding the stateroot /var (--defer-var), so that the
	// image's /var content is stored once. Only used with Initialize.
	Defer bool
}

// NewBootcInstallToFilesystem creates a new stage for the
// org.osbuild.bootc.install-to-filesystem stage.
//
// It requires a root mount, and on UEFI platforms also an ESP mount so
// bootupd can run. Aboot callers pass a platform without a UEFI vendor:
// bootc discovers and mounts an ESP itself for ukiboot.
// With varMounts.Initialize, filesystems for /var and below are mounted as
// well, and bootc is asked to initialize them from the image, and with
// varMounts.Defer also not to seed the stateroot /var. Only set them for a
// bootc that advertises this (see bootc.Info.InstallVarMounts and
// bootc.Info.InstallDeferVar).
//
// The mounts input should be generated with GenBootupdDevicesMounts.
func NewBootcInstallToFilesystemStage(options *BootcInstallToFilesystemOptions, inputs ContainerDeployInputs, devices map[string]Device, mounts []Mount, pltf platform.Platform, varMounts BootcInstallVarMounts) (*Stage, error) {
	if err := validateBootupdMounts(mounts, pltf); err != nil {
		return nil, err
	}

	if len(inputs.Images.References) != 1 {
		return nil, fmt.Errorf("expected exactly one container input but got: %v (%v)", len(inputs.Images.References), inputs.Images.References)
	}

	// Don't mount other custom mountpoints, bootc requires an otherwise empty
	// target. Older bootc versions reject /var mountpoints, or leave those
	// filesystems empty so that they hide the image's /var content at boot.
	requiredMountpoints := []string{"/", "/boot", "/boot/efi"}
	reqMounts := make([]Mount, 0, len(mounts))
	hasVarMounts := false
	for _, mount := range mounts {
		isVarMount := varMounts.Initialize && IsVarMountpoint(mount.Target)
		if slices.Contains(requiredMountpoints, mount.Target) || isVarMount {
			reqMounts = append(reqMounts, mount)
		}
		hasVarMounts = hasVarMounts || isVarMount
	}
	// The filesystems we just created are empty, and bootc leaves the ones it
	// did not mount itself alone unless asked to initialize them. Without
	// /var filesystems, /var is seeded as before, even if bootc could defer it.
	if hasVarMounts {
		opts := BootcInstallToFilesystemOptions{}
		if options != nil {
			opts = *options
		}
		opts.InitializeVarMounts = true
		opts.DeferVar = varMounts.Defer
		options = &opts
	}

	return &Stage{
		Type:    "org.osbuild.bootc.install-to-filesystem",
		Options: options,
		Inputs:  inputs,
		Devices: devices,
		Mounts:  reqMounts,
	}, nil
}

// IsVarMountpoint returns true for /var and mountpoints below it.
func IsVarMountpoint(target string) bool {
	return target == "/var" || strings.HasPrefix(target, "/var/")
}
