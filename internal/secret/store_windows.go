//go:build windows

package secret

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const cryptProtectUIForbidden = 0x1

func protect(value []byte) ([]byte, error) {
	if len(value) == 0 {
		return nil, fmt.Errorf("cannot protect an empty credential")
	}
	in := windows.DataBlob{Size: uint32(len(value)), Data: &value[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, cryptProtectUIForbidden, &out); err != nil {
		return nil, fmt.Errorf("protect credential with Windows DPAPI: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

func unprotect(value []byte) ([]byte, error) {
	if len(value) == 0 {
		return nil, fmt.Errorf("encrypted credential is empty")
	}
	in := windows.DataBlob{Size: uint32(len(value)), Data: &value[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, cryptProtectUIForbidden, &out); err != nil {
		return nil, fmt.Errorf("unprotect credential with Windows DPAPI: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
