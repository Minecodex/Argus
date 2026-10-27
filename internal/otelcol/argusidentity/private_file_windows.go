//go:build windows

package argusidentity

import (
	"errors"
	"golang.org/x/sys/windows"
	"maps"
	"unsafe"
)

func privateFileDescriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;" + user.User.Sid.String() + ")")
}

func protectPrivateFile(path string) error {
	descriptor, err := privateFileDescriptor()
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

func verifyPrivateFile(path string) error {
	actual, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	expected, err := privateFileDescriptor()
	if err != nil {
		return err
	}
	if actual == nil {
		return errors.New("identity file DACL is not private")
	}
	control, _, err := actual.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("identity file inherits access")
	}
	actualEntries, err := privateFileEntries(actual)
	if err != nil {
		return err
	}
	expectedEntries, err := privateFileEntries(expected)
	if err != nil {
		return err
	}
	if !maps.Equal(actualEntries, expectedEntries) {
		return errors.New("identity file DACL is not private")
	}
	return nil
}

func privateFileEntries(descriptor *windows.SECURITY_DESCRIPTOR) (map[string]windows.ACCESS_MASK, error) {
	acl, _, err := descriptor.DACL()
	if err != nil || acl == nil {
		return nil, errors.New("identity file has no restrictive DACL")
	}
	entries := map[string]windows.ACCESS_MASK{}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(acl, i, &ace); err != nil {
			return nil, err
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != 0 {
			return nil, errors.New("unexpected identity file access rule")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		entries[sid.String()] |= ace.Mask
	}
	return entries, nil
}
