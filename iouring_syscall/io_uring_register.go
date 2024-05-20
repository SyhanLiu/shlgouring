//go:build linux

package iouring_syscall

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

func IOURingRegister(fd int, opcode uint8, args unsafe.Pointer, nrArgs uint32) error {
	for {
		_, _, errno := syscall.RawSyscall6(SYS_IO_URING_REGISTER, uintptr(fd), uintptr(opcode), uintptr(args),
			uintptr(nrArgs), 0, 0)
		if errno != 0 {
			// EINTR may be returned when blocked
			if errors.Is(errno, syscall.EINTR) {
				continue
			}
			return os.NewSyscallError("iouring_register", errno)
		}
		return nil
	}
}
