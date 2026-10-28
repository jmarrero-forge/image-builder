package osbuild_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/osbuild/image-builder/pkg/arch"
	"github.com/osbuild/image-builder/pkg/container"
	"github.com/osbuild/image-builder/pkg/osbuild"
	"github.com/osbuild/image-builder/pkg/platform"
)

func makeFakeContainerInputs() osbuild.ContainerDeployInputs {
	return osbuild.ContainerDeployInputs{
		Images: osbuild.NewContainersInputForSources([]container.Spec{
			{
				ImageID:   "id-0",
				Source:    "registry.example.org/reg/img",
				LocalName: "local-name",
			},
		},
		),
	}
}

func TestBootcInstallToFilesystemStageNewHappy(t *testing.T) {
	devices := makeOsbuildDevices("dev-for-/", "dev-for-/boot", "dev-for-/boot/efi")
	mounts := makeOsbuildMounts("/", "/boot", "/boot/efi")
	inputs := makeFakeContainerInputs()
	pf := &platform.Data{
		Arch:       arch.ARCH_X86_64,
		UEFIVendor: "test",
	}

	expectedStage := &osbuild.Stage{
		Type:    "org.osbuild.bootc.install-to-filesystem",
		Options: (*osbuild.BootcInstallToFilesystemOptions)(nil),
		Inputs:  inputs,
		Devices: devices,
		Mounts:  mounts,
	}
	stage, err := osbuild.NewBootcInstallToFilesystemStage(nil, inputs, devices, mounts, pf, osbuild.BootcInstallVarMounts{})
	require.Nil(t, err)
	assert.Equal(t, stage, expectedStage)
}

func TestBootcInstallToFilesystemStageWithoutUEFIVendor(t *testing.T) {
	pf := &platform.Data{Arch: arch.ARCH_X86_64}
	_, err := osbuild.NewBootcInstallToFilesystemStage(nil, makeFakeContainerInputs(), nil, makeOsbuildMounts("/"), pf, osbuild.BootcInstallVarMounts{})
	require.NoError(t, err)
	_, err = osbuild.NewBootcInstallToFilesystemStage(nil, makeFakeContainerInputs(), nil, nil, pf, osbuild.BootcInstallVarMounts{})
	require.ErrorContains(t, err, "required mounts for bootupd stage [/] missing")
}

func TestBootcInstallToFilesystemStageNewVarMounts(t *testing.T) {
	devices := makeOsbuildDevices("dev-for-/", "dev-for-/boot/efi", "dev-for-/var", "dev-for-/var/log", "dev-for-/var/log/audit")
	mounts := makeOsbuildMounts("/", "/boot/efi", "/var", "/var/log", "/var/log/audit", "/opt", "/tmp", "/variable")
	inputs := makeFakeContainerInputs()
	pf := &platform.Data{
		Arch:       arch.ARCH_X86_64,
		UEFIVendor: "test",
	}

	bootMounts := []osbuild.Mount{
		{Name: "mnt-for-/", Type: "org.osbuild.ext4", Source: "dev-for-/", Target: "/"},
		{Name: "mnt-for-/boot/efi", Type: "org.osbuild.vfat", Source: "dev-for-/boot/efi", Target: "/boot/efi"},
	}
	varMounts := []osbuild.Mount{
		{Name: "mnt-for-/var", Type: "org.osbuild.ext4", Source: "dev-for-/var", Target: "/var"},
		{Name: "mnt-for-/var/log", Type: "org.osbuild.ext4", Source: "dev-for-/var/log", Target: "/var/log"},
		{Name: "mnt-for-/var/log/audit", Type: "org.osbuild.ext4", Source: "dev-for-/var/log/audit", Target: "/var/log/audit"},
	}
	allMounts := append(slices.Clone(bootMounts), varMounts...)
	for _, tc := range []struct {
		varMounts       osbuild.BootcInstallVarMounts
		expectedOptions *osbuild.BootcInstallToFilesystemOptions
		expectedMounts  []osbuild.Mount
	}{
		{osbuild.BootcInstallVarMounts{}, nil, bootMounts},
		// --defer-var only comes with the /var filesystems
		{osbuild.BootcInstallVarMounts{Defer: true}, nil, bootMounts},
		// bootc is asked to initialize the /var filesystems it gets
		{osbuild.BootcInstallVarMounts{Initialize: true}, &osbuild.BootcInstallToFilesystemOptions{InitializeVarMounts: true}, allMounts},
		{osbuild.BootcInstallVarMounts{Initialize: true, Defer: true}, &osbuild.BootcInstallToFilesystemOptions{InitializeVarMounts: true, DeferVar: true}, allMounts},
	} {
		t.Run(fmt.Sprintf("%+v", tc.varMounts), func(t *testing.T) {
			expectedStage := &osbuild.Stage{
				Type:    "org.osbuild.bootc.install-to-filesystem",
				Options: tc.expectedOptions,
				Inputs:  inputs,
				Devices: devices,
				Mounts:  tc.expectedMounts,
			}
			stage, err := osbuild.NewBootcInstallToFilesystemStage(nil, inputs, devices, mounts, pf, tc.varMounts)
			require.Nil(t, err)
			assert.Equal(t, expectedStage, stage)
		})
	}
}

func TestBootcInstallToFilesystemStageVarMountsKeepOptions(t *testing.T) {
	devices := makeOsbuildDevices("dev-for-/", "dev-for-/boot/efi", "dev-for-/var/log")
	mounts := makeOsbuildMounts("/", "/boot/efi", "/var/log")
	pf := &platform.Data{Arch: arch.ARCH_X86_64, UEFIVendor: "test"}
	opts := &osbuild.BootcInstallToFilesystemOptions{TargetImgref: "quay.io/example/img"}

	stage, err := osbuild.NewBootcInstallToFilesystemStage(opts, makeFakeContainerInputs(), devices, mounts, pf, osbuild.BootcInstallVarMounts{Initialize: true, Defer: true})
	require.NoError(t, err)
	assert.Equal(t, &osbuild.BootcInstallToFilesystemOptions{
		TargetImgref:        "quay.io/example/img",
		InitializeVarMounts: true,
		DeferVar:            true,
	}, stage.Options)
	// The caller's options are not modified
	assert.False(t, opts.InitializeVarMounts)
	assert.False(t, opts.DeferVar)
}

func TestBootcInstallToFilesystemStageNoVarMounts(t *testing.T) {
	// A bootc that can initialize /var filesystems is not asked to without
	// any, nor to defer /var
	devices := makeOsbuildDevices("dev-for-/", "dev-for-/boot/efi")
	mounts := makeOsbuildMounts("/", "/boot/efi")
	pf := &platform.Data{Arch: arch.ARCH_X86_64, UEFIVendor: "test"}
	stage, err := osbuild.NewBootcInstallToFilesystemStage(nil, makeFakeContainerInputs(), devices, mounts, pf, osbuild.BootcInstallVarMounts{Initialize: true, Defer: true})
	require.NoError(t, err)
	assert.Nil(t, stage.Options)
}

func TestIsVarMountpoint(t *testing.T) {
	for target, expected := range map[string]bool{
		"/var":         true,
		"/var/":        true,
		"/var/log":     true,
		"/var/lib/app": true,
		"/":            false,
		"/variable":    false,
		"/opt/var":     false,
		"/boot/efi":    false,
	} {
		assert.Equal(t, expected, osbuild.IsVarMountpoint(target), target)
	}
}

func TestBootcInstallToFilesystemStageNewNoContainers(t *testing.T) {
	devices := makeOsbuildDevices("dev-for-/", "dev-for-/boot", "dev-for-/boot/efi")
	mounts := makeOsbuildMounts("/", "/boot", "/boot/efi")
	inputs := osbuild.ContainerDeployInputs{}
	pf := &platform.Data{
		Arch:       arch.ARCH_X86_64,
		UEFIVendor: "test",
	}

	_, err := osbuild.NewBootcInstallToFilesystemStage(nil, inputs, devices, mounts, pf, osbuild.BootcInstallVarMounts{})
	assert.EqualError(t, err, "expected exactly one container input but got: 0 (map[])")
}

func TestBootcInstallToFilesystemStageNewTwoContainers(t *testing.T) {
	devices := makeOsbuildDevices("dev-for-/", "dev-for-/boot", "dev-for-/boot/efi")
	mounts := makeOsbuildMounts("/", "/boot", "/boot/efi")
	inputs := osbuild.ContainerDeployInputs{
		Images: osbuild.ContainersInput{
			References: map[string]osbuild.ContainersInputSourceRef{
				"1": {},
				"2": {},
			},
		},
	}
	pf := &platform.Data{
		Arch:       arch.ARCH_X86_64,
		UEFIVendor: "test",
	}

	_, err := osbuild.NewBootcInstallToFilesystemStage(nil, inputs, devices, mounts, pf, osbuild.BootcInstallVarMounts{})
	assert.EqualError(t, err, "expected exactly one container input but got: 2 (map[1:{} 2:{}])")
}

func TestBootcInstallToFilesystemStageMissingMounts(t *testing.T) {
	devices := makeOsbuildDevices("dev-for-/")
	mounts := makeOsbuildMounts("/")
	inputs := makeFakeContainerInputs()
	pf := &platform.Data{
		Arch:       arch.ARCH_X86_64,
		UEFIVendor: "test",
	}

	stage, err := osbuild.NewBootcInstallToFilesystemStage(nil, inputs, devices, mounts, pf, osbuild.BootcInstallVarMounts{})
	// XXX: rename error
	assert.ErrorContains(t, err, "required mounts for bootupd stage [/boot/efi] missing")
	require.Nil(t, stage)
}

func TestBootcInstallToFilesystemStageJsonHappy(t *testing.T) {
	devices := makeOsbuildDevices("disk", "dev-for-/", "dev-for-/boot", "dev-for-/boot/efi")
	mounts := makeOsbuildMounts("/", "/boot", "/boot/efi")
	inputs := makeFakeContainerInputs()
	pf := &platform.Data{
		Arch:       arch.ARCH_X86_64,
		UEFIVendor: "test",
	}

	opts := &osbuild.BootcInstallToFilesystemOptions{
		TargetImgref: "quay.io/centos-bootc/centos-bootc-dev:stream9",
	}
	stage, err := osbuild.NewBootcInstallToFilesystemStage(opts, inputs, devices, mounts, pf, osbuild.BootcInstallVarMounts{})
	require.Nil(t, err)
	stageJson, err := json.MarshalIndent(stage, "", "  ")
	require.Nil(t, err)
	assert.Equal(t, string(stageJson), `{
  "type": "org.osbuild.bootc.install-to-filesystem",
  "inputs": {
    "images": {
      "type": "org.osbuild.containers",
      "origin": "org.osbuild.source",
      "references": {
        "id-0": {
          "name": "local-name"
        }
      }
    }
  },
  "options": {
    "target-imgref": "quay.io/centos-bootc/centos-bootc-dev:stream9"
  },
  "devices": {
    "dev-for-/": {
      "type": "org.osbuild.loopback"
    },
    "dev-for-/boot": {
      "type": "org.osbuild.loopback"
    },
    "dev-for-/boot/efi": {
      "type": "org.osbuild.loopback"
    },
    "disk": {
      "type": "org.osbuild.loopback"
    }
  },
  "mounts": [
    {
      "name": "mnt-for-/",
      "type": "org.osbuild.ext4",
      "source": "dev-for-/",
      "target": "/"
    },
    {
      "name": "mnt-for-/boot",
      "type": "org.osbuild.ext4",
      "source": "dev-for-/boot",
      "target": "/boot"
    },
    {
      "name": "mnt-for-/boot/efi",
      "type": "org.osbuild.vfat",
      "source": "dev-for-/boot/efi",
      "target": "/boot/efi"
    }
  ]
}`)
}

func TestBootcInstallToFilesystemOptionsVarJSON(t *testing.T) {
	// The keys are the options of osbuild's org.osbuild.bootc.install-to-filesystem stage
	for _, tc := range []struct {
		opts     osbuild.BootcInstallToFilesystemOptions
		expected string
	}{
		{osbuild.BootcInstallToFilesystemOptions{}, `{"target-imgref":""}`},
		{osbuild.BootcInstallToFilesystemOptions{InitializeVarMounts: true, DeferVar: true}, `{"target-imgref":"","initialize-var-mounts":true,"defer-var":true}`},
	} {
		data, err := json.Marshal(tc.opts)
		require.NoError(t, err)
		assert.JSONEq(t, tc.expected, string(data))
	}
}
