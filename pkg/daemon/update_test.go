package daemon

import (
	"fmt"
	"testing"

	ignv2_2types "github.com/coreos/ignition/config/v2_2/types"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	"github.com/openshift/machine-config-operator/pkg/generated/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestUpdateOS verifies the return errors from attempting to update the OS follow expectations
func TestUpdateOS(t *testing.T) {
	// expectedError is the error we will use when expecting an error to return
	expectedError := fmt.Errorf("broken")

	// testClient is the NodeUpdaterClient mock instance that will front
	// calls to update the host.
	testClient := RpmOstreeClientMock{
		GetBootedOSImageURLReturns: []GetBootedOSImageURLReturn{},
		RunPivotReturns: []error{
			// First run will return no error
			nil,
			// Second rrun will return our expected error
			expectedError},
	}

	// Create a Daemon instance with mocked clients
	d := Daemon{
		name:              "nodeName",
		OperatingSystem:   machineConfigDaemonOSRHCOS,
		NodeUpdaterClient: testClient,
		loginClient:       nil, // set to nil as it will not be used within tests
		client:            fake.NewSimpleClientset(),
		kubeClient:        k8sfake.NewSimpleClientset(),
		rootMount:         "/",
		bootedOSImageURL:  "test",
	}

	// Set up machineconfigs to pass to updateOS.
	mcfg := &mcfgv1.MachineConfig{}
	// differentMcfg has a different OSImageURL so it will force Daemon.UpdateOS
	// to trigger an update of the operatingsystem (as fronted by our testClient)
	differentMcfg := &mcfgv1.MachineConfig{
		Spec: mcfgv1.MachineConfigSpec{
			OSImageURL: "somethingDifferent",
		},
	}

	// This should be a no-op
	if err := d.updateOS(mcfg); err != nil {
		t.Errorf("Expected no error. Got %s.", err)
	}
	// Second call should return an error
	if err := d.updateOS(differentMcfg); err == expectedError {
		t.Error("Expected an error. Got none.")
	}
}

// TestReconcilable attempts to verify the conditions in which configs would and would not be
// reconcilable. Welcome to the longest unittest you've ever read.
func TestReconcilable(t *testing.T) {
	d := Daemon{
		name:              "nodeName",
		OperatingSystem:   machineConfigDaemonOSRHCOS,
		NodeUpdaterClient: nil,
		loginClient:       nil,
		client:            nil,
		kubeClient:        nil,
		rootMount:         "/",
		bootedOSImageURL:  "test",
	}

	// oldConfig is the current config of the fake system
	oldConfig := &mcfgv1.MachineConfig{
		Spec: mcfgv1.MachineConfigSpec{
			Config: ignv2_2types.Config{
				Ignition: ignv2_2types.Ignition{
					Version: "2.0.0",
				},
			},
		},
	}

	// newConfig is the config that is being requested to apply to the system
	newConfig := &mcfgv1.MachineConfig{
		Spec: mcfgv1.MachineConfigSpec{
			Config: ignv2_2types.Config{
				Ignition: ignv2_2types.Ignition{
					Version: "2.2.0",
				},
			},
		},
	}

	// Verify Ignition version changes react as expected
	isReconcilable := d.reconcilable(oldConfig, newConfig)
	checkIrreconcilableResults(t, "Ignition", isReconcilable)

	// Match ignition versions
	oldConfig.Spec.Config.Ignition.Version = "2.2.0"
	isReconcilable = d.reconcilable(oldConfig, newConfig)
	checkReconcilableResults(t, "Ignition", isReconcilable)

	// Verify Networkd unit changes react as expected
	oldConfig.Spec.Config.Networkd = ignv2_2types.Networkd{}
	newConfig.Spec.Config.Networkd = ignv2_2types.Networkd{
		Units: []ignv2_2types.Networkdunit{
			ignv2_2types.Networkdunit{
				Name: "test.network",
			},
		},
	}
	isReconcilable = d.reconcilable(oldConfig, newConfig)
	checkIrreconcilableResults(t, "Networkd", isReconcilable)

	// Match Networkd
	oldConfig.Spec.Config.Networkd = newConfig.Spec.Config.Networkd

	isReconcilable = d.reconcilable(oldConfig, newConfig)
	checkReconcilableResults(t, "Networkd", isReconcilable)

	// Verify Disk changes react as expected
	oldConfig.Spec.Config.Storage.Disks = []ignv2_2types.Disk{
		ignv2_2types.Disk{
			Device: "/one",
		},
	}

	isReconcilable = d.reconcilable(oldConfig, newConfig)
	checkIrreconcilableResults(t, "Disk", isReconcilable)

	// Match storage disks
	newConfig.Spec.Config.Storage.Disks = oldConfig.Spec.Config.Storage.Disks
	isReconcilable = d.reconcilable(oldConfig, newConfig)
	checkReconcilableResults(t, "Disk", isReconcilable)

	// Verify Filesystems changes react as expected
	oldFSPath := "/foo/bar"
	oldConfig.Spec.Config.Storage.Filesystems = []ignv2_2types.Filesystem{
		ignv2_2types.Filesystem{
			Name: "user",
			Path: &oldFSPath,
		},
	}

	isReconcilable = d.reconcilable(oldConfig, newConfig)
	checkIrreconcilableResults(t, "Filesystem", isReconcilable)

	// Match Storage filesystems
	newConfig.Spec.Config.Storage.Filesystems = oldConfig.Spec.Config.Storage.Filesystems
	isReconcilable = d.reconcilable(oldConfig, newConfig)
	checkReconcilableResults(t, "Filesystem", isReconcilable)

	// Verify Raid changes react as expected
	oldConfig.Spec.Config.Storage.Raid = []ignv2_2types.Raid{
		ignv2_2types.Raid{
			Name:  "data",
			Level: "stripe",
		},
	}

	isReconcilable = d.reconcilable(oldConfig, newConfig)
	checkIrreconcilableResults(t, "Raid", isReconcilable)

	// Match storage raid
	newConfig.Spec.Config.Storage.Raid = oldConfig.Spec.Config.Storage.Raid
	isReconcilable = d.reconcilable(oldConfig, newConfig)
	checkReconcilableResults(t, "Raid", isReconcilable)

	// Verify Passwd Groups changes unsupported
	oldConfig = &mcfgv1.MachineConfig{}
	tempGroup := ignv2_2types.PasswdGroup{Name: "testGroup"}
	newMcfg := &mcfgv1.MachineConfig{
		Spec: mcfgv1.MachineConfigSpec{
			Config: ignv2_2types.Config{
				Passwd: ignv2_2types.Passwd{
					Groups: []ignv2_2types.PasswdGroup{tempGroup},
				},
			},
		},
	}
	isReconcilable = d.reconcilable(oldConfig, newMcfg)
	checkIrreconcilableResults(t, "PasswdGroups", isReconcilable)

}

func TestReconcilableSSH(t *testing.T) {
	// expectedError is the error we will use when expecting an error to return
	expectedError := fmt.Errorf("broken")

	// testClient is the NodeUpdaterClient mock instance that will front
	// calls to update the host.
	testClient := RpmOstreeClientMock{
		GetBootedOSImageURLReturns: []GetBootedOSImageURLReturn{},
		RunPivotReturns: []error{
			// First run will return no error
			nil,
			// Second rrun will return our expected error
			expectedError},
	}

	// Create a Daemon instance with mocked clients
	d := Daemon{
		name:              "nodeName",
		OperatingSystem:   machineConfigDaemonOSRHCOS,
		NodeUpdaterClient: testClient,
		loginClient:       nil, // set to nil as it will not be used within tests
		client:            fake.NewSimpleClientset(),
		kubeClient:        k8sfake.NewSimpleClientset(),
		rootMount:         "/",
		bootedOSImageURL:  "test",
	}

	// Check that updating SSH Key of user core supported
	//tempUser1 := ignv2_2types.PasswdUser{Name: "core", SSHAuthorizedKeys: []ignv2_2types.SSHAuthorizedKey{"1234"}}
	oldMcfg := &mcfgv1.MachineConfig{
		Spec: mcfgv1.MachineConfigSpec{
			Config: ignv2_2types.Config{
				Ignition: ignv2_2types.Ignition{
					Version: "2.2.0",
				},
			},
		},
	}
	tempUser1 := ignv2_2types.PasswdUser{Name: "core", SSHAuthorizedKeys: []ignv2_2types.SSHAuthorizedKey{"5678", "abc"}}
	newMcfg := &mcfgv1.MachineConfig{
		Spec: mcfgv1.MachineConfigSpec{
			Config: ignv2_2types.Config{
				Ignition: ignv2_2types.Ignition{
					Version: "2.2.0",
				},
				Passwd: ignv2_2types.Passwd{
					Users: []ignv2_2types.PasswdUser{tempUser1},
				},
			},
		},
	}

	errMsg := d.reconcilable(oldMcfg, newMcfg)
	checkReconcilableResults(t, "SSH", errMsg)

	// 	Check that updating User with User that is not core is not supported
	tempUser2 := ignv2_2types.PasswdUser{Name: "core", SSHAuthorizedKeys: []ignv2_2types.SSHAuthorizedKey{"1234"}}
	oldMcfg.Spec.Config.Passwd.Users = append(oldMcfg.Spec.Config.Passwd.Users, tempUser2)
	tempUser3 := ignv2_2types.PasswdUser{Name: "another user", SSHAuthorizedKeys: []ignv2_2types.SSHAuthorizedKey{"5678"}}
	newMcfg.Spec.Config.Passwd.Users[0] = tempUser3

	errMsg = d.reconcilable(oldMcfg, newMcfg)
	checkIrreconcilableResults(t, "SSH", errMsg)

	// check that we cannot make updates if any other Passwd.User field is changed.
	tempUser4 := ignv2_2types.PasswdUser{Name: "core", SSHAuthorizedKeys: []ignv2_2types.SSHAuthorizedKey{"5678"}, HomeDir: "somedir"}
	newMcfg.Spec.Config.Passwd.Users[0] = tempUser4

	errMsg = d.reconcilable(oldMcfg, newMcfg)
	checkIrreconcilableResults(t, "SSH", errMsg)

	// check that we cannot add a user or have len(Passwd.Users)> 1
	tempUser5 := ignv2_2types.PasswdUser{Name: "some user", SSHAuthorizedKeys: []ignv2_2types.SSHAuthorizedKey{"5678"}}
	newMcfg.Spec.Config.Passwd.Users = append(newMcfg.Spec.Config.Passwd.Users, tempUser5)

	errMsg = d.reconcilable(oldMcfg, newMcfg)
	checkIrreconcilableResults(t, "SSH", errMsg)

	// check that user is not attempting to remove the only sshkey from core user
	tempUser6 := ignv2_2types.PasswdUser{Name: "core", SSHAuthorizedKeys: []ignv2_2types.SSHAuthorizedKey{}}
	newMcfg.Spec.Config.Passwd.Users[0] = tempUser6
	newMcfg.Spec.Config.Passwd.Users = newMcfg.Spec.Config.Passwd.Users[:len(newMcfg.Spec.Config.Passwd.Users)-1]

	errMsg = d.reconcilable(oldMcfg, newMcfg)
	checkIrreconcilableResults(t, "SSH", errMsg)

	//check that empty Users does not generate error/degrade node
	newMcfg.Spec.Config.Passwd.Users = nil

	errMsg = d.reconcilable(oldMcfg, newMcfg)
	checkReconcilableResults(t, "SSH", errMsg)

}

func TestUpdateSSHKeys(t *testing.T) {
	// expectedError is the error we will use when expecting an error to return
	expectedError := fmt.Errorf("broken")
	// testClient is the NodeUpdaterClient mock instance that will front
	// calls to update the host.
	testClient := RpmOstreeClientMock{
		GetBootedOSImageURLReturns: []GetBootedOSImageURLReturn{},
		RunPivotReturns: []error{
			// First run will return no error
			nil,
			// Second rrun will return our expected error
			expectedError},
	}
	mockFS := &FsClientMock{MkdirAllReturns: []error{nil}, WriteFileReturns: []error{nil}}
	// Create a Daemon instance with mocked clients
	d := Daemon{
		name:              "nodeName",
		OperatingSystem:   machineConfigDaemonOSRHCOS,
		NodeUpdaterClient: testClient,
		loginClient:       nil, // set to nil as it will not be used within tests
		client:            fake.NewSimpleClientset(),
		kubeClient:        k8sfake.NewSimpleClientset(),
		rootMount:         "/",
		bootedOSImageURL:  "test",
		fileSystemClient:  mockFS,
	}
	// Set up machineconfigs that are identical except for SSH keys
	tempUser := ignv2_2types.PasswdUser{Name: "core", SSHAuthorizedKeys: []ignv2_2types.SSHAuthorizedKey{"1234", "4567"}}

	newMcfg := &mcfgv1.MachineConfig{
		Spec: mcfgv1.MachineConfigSpec{
			Config: ignv2_2types.Config{
				Passwd: ignv2_2types.Passwd{
					Users: []ignv2_2types.PasswdUser{tempUser},
				},
			},
		},
	}
	err := d.updateSSHKeys(newMcfg.Spec.Config.Passwd.Users)
	if err != nil {
		t.Errorf("Expected no error. Got %s.", err)

	}

	// if Users is empty, nothing should happen and no error should ever be generated
	newMcfg2 := &mcfgv1.MachineConfig{}
	err = d.updateSSHKeys(newMcfg2.Spec.Config.Passwd.Users)
	if err != nil {
		t.Errorf("Expected no error. Got: %s", err)
	}
}

// This test should fail until Ignition validation enabled.
// Ignition validation does not permit writing files to relative paths.
func TestInvalidIgnConfig(t *testing.T) {
	// expectedError is the error we will use when expecting an error to return
	expectedError := fmt.Errorf("broken")
	// testClient is the NodeUpdaterClient mock instance that will front
	// calls to update the host.
	testClient := RpmOstreeClientMock{
		GetBootedOSImageURLReturns: []GetBootedOSImageURLReturn{},
		RunPivotReturns: []error{
			// First run will return no error
			nil,
			// Second rrun will return our expected error
			expectedError},
	}
	mockFS := &FsClientMock{MkdirAllReturns: []error{nil}, WriteFileReturns: []error{nil}}
	// Create a Daemon instance with mocked clients
	d := Daemon{
		name:              "nodeName",
		OperatingSystem:   machineConfigDaemonOSRHCOS,
		NodeUpdaterClient: testClient,
		loginClient:       nil, // set to nil as it will not be used within tests
		client:            fake.NewSimpleClientset(),
		kubeClient:        k8sfake.NewSimpleClientset(),
		rootMount:         "/",
		bootedOSImageURL:  "test",
		fileSystemClient:  mockFS,
	}

	oldMcfg := &mcfgv1.MachineConfig{
		Spec: mcfgv1.MachineConfigSpec{
			Config: ignv2_2types.Config{
				Ignition: ignv2_2types.Ignition{
					Version: "2.2.0",
				},
			},
		},
	}
	// create file to write that contains an impermissable relative path
	tempFileContents := ignv2_2types.FileContents{Source: "data:,hello%20world%0A"}
	tempMode := 420
	newMcfg := &mcfgv1.MachineConfig{
		Spec: mcfgv1.MachineConfigSpec{
			Config: ignv2_2types.Config{
				Ignition: ignv2_2types.Ignition{
					Version: "2.2.0",
				},
				Storage: ignv2_2types.Storage{
					Files: []ignv2_2types.File{
						{Node: ignv2_2types.Node{Path: "home/core/test", Filesystem: "root"},
							FileEmbedded1: ignv2_2types.FileEmbedded1{Contents: tempFileContents, Mode: &tempMode}},
					},
				},
			},
		},
	}
	err := d.reconcilable(oldMcfg, newMcfg)
	assert.NotNil(t, err, "Expected error. Relative Paths should fail general ignition validation")

	newMcfg.Spec.Config.Storage.Files[0].Node.Path = "/home/core/test"
	err = d.reconcilable(oldMcfg, newMcfg)
	assert.Nil(t, err, "Expected no error. Absolute paths should not fail general ignition validation")

}

// checkReconcilableResults is a shortcut for verifying results that should be reconcilable
func checkReconcilableResults(t *testing.T, key string, reconcilableError error) {
	if reconcilableError != nil {
		t.Errorf("%s values should be reconcilable. Received error: %v", key, reconcilableError)
	}
}

// checkIrreconcilableResults is a shortcut for verifing results that should be irreconcilable
func checkIrreconcilableResults(t *testing.T, key string, reconcilableError error) {
	if reconcilableError == nil {
		t.Errorf("Different %s values should not be reconcilable.", key)
	}
}
