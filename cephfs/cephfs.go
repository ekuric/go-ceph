package cephfs

/*
#cgo LDFLAGS: -lcephfs
#cgo CPPFLAGS: -D_FILE_OFFSET_BITS=64
#include <stdlib.h>
#include <cephfs/libcephfs.h>
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/ceph/go-ceph/errutil"
	"github.com/ceph/go-ceph/rados"
)

// CephFSError represents an error condition returned from the CephFS APIs.
type CephFSError int

// Error returns the error string for the CephFSError type.
func (e CephFSError) Error() string {
	errno, s := errutil.FormatErrno(int(e))
	if s == "" {
		return fmt.Sprintf("cephfs: ret=%d", errno)
	}
	return fmt.Sprintf("cephfs: ret=%d, %s", errno, s)
}

func getError(e C.int) error {
	if e == 0 {
		return nil
	}
	return CephFSError(e)
}

// MountInfo exports ceph's ceph_mount_info from libcephfs.cc
type MountInfo struct {
	mount *C.struct_ceph_mount_info
}

func createMount(id *C.char) (*MountInfo, error) {
	mount := &MountInfo{}
	ret := C.ceph_create(&mount.mount, id)
	if ret != 0 {
		return nil, getError(ret)
	}
	return mount, nil
}

// CreateMount creates a mount handle for interacting with Ceph.
func CreateMount() (*MountInfo, error) {
	return createMount(nil)
}

// CreateMountWithId creates a mount handle for interacting with Ceph.
// The caller can specify a unique id that will identify this client.
func CreateMountWithId(id string) (*MountInfo, error) {
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	return createMount(cid)
}

// CreateFromRados creates a mount handle using an existing rados cluster
// connection.
//
// Implements:
//  int ceph_create_from_rados(struct ceph_mount_info **cmount, rados_t cluster);
func CreateFromRados(conn *rados.Conn) (*MountInfo, error) {
	mount := &MountInfo{}
	ret := C.ceph_create_from_rados(&mount.mount, C.rados_t(conn.Cluster()))
	if ret != 0 {
		return nil, getError(ret)
	}
	return mount, nil
}

// ReadDefaultConfigFile loads the ceph configuration from the specified config file.
func (mount *MountInfo) ReadDefaultConfigFile() error {
	ret := C.ceph_conf_read_file(mount.mount, nil)
	return getError(ret)
}

// Mount the file system, establishing a connection capable of I/O.
//
// Implements:
//  int ceph_mount(struct ceph_mount_info *cmount, const char *root);
func (mount *MountInfo) Mount() error {
	ret := C.ceph_mount(mount.mount, nil)
	return getError(ret)
}

// Unmount the file system.
//
// Implements:
//  int ceph_unmount(struct ceph_mount_info *cmount);
func (mount *MountInfo) Unmount() error {
	ret := C.ceph_unmount(mount.mount)
	return getError(ret)
}

// Release destroys the mount handle.
//
// Implements:
//  int ceph_release(struct ceph_mount_info *cmount);
func (mount *MountInfo) Release() error {
	ret := C.ceph_release(mount.mount)
	return getError(ret)
}

// SyncFs synchronizes all filesystem data to persistent media.
func (mount *MountInfo) SyncFs() error {
	ret := C.ceph_sync_fs(mount.mount)
	return getError(ret)
}

// CurrentDir gets the current working directory.
func (mount *MountInfo) CurrentDir() string {
	cDir := C.ceph_getcwd(mount.mount)
	return C.GoString(cDir)
}

// ChangeDir changes the current working directory.
func (mount *MountInfo) ChangeDir(path string) error {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	ret := C.ceph_chdir(mount.mount, cPath)
	return getError(ret)
}

// MakeDir creates a directory.
func (mount *MountInfo) MakeDir(path string, mode uint32) error {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	ret := C.ceph_mkdir(mount.mount, cPath, C.mode_t(mode))
	return getError(ret)
}

// RemoveDir removes a directory.
func (mount *MountInfo) RemoveDir(path string) error {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	ret := C.ceph_rmdir(mount.mount, cPath)
	return getError(ret)
}

// Chmod changes the mode bits (permissions) of a file/directory.
func (mount *MountInfo) Chmod(path string, mode uint32) error {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	ret := C.ceph_chmod(mount.mount, cPath, C.mode_t(mode))
	return getError(ret)
}

// Chown changes the ownership of a file/directory.
func (mount *MountInfo) Chown(path string, user uint32, group uint32) error {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	ret := C.ceph_chown(mount.mount, cPath, C.int(user), C.int(group))
	return getError(ret)
}

// IsMounted checks mount status.
func (mount *MountInfo) IsMounted() bool {
	ret := C.ceph_is_mounted(mount.mount)
	return ret == 1
}

// MdsCommand sends commands to the specified MDS.
func (mount *MountInfo) MdsCommand(mdsSpec string, args [][]byte) ([]byte, string, error) {
	return mount.mdsCommand(mdsSpec, args, nil)
}

// MdsCommandWithInputBuffer sends commands to the specified MDS, with an input
// buffer.
func (mount *MountInfo) MdsCommandWithInputBuffer(mdsSpec string, args [][]byte, inputBuffer []byte) ([]byte, string, error) {
	return mount.mdsCommand(mdsSpec, args, inputBuffer)
}

// mdsCommand supports sending formatted commands to MDS.
//
// Implements:
//  int ceph_mds_command(struct ceph_mount_info *cmount,
//      const char *mds_spec,
//      const char **cmd,
//      size_t cmdlen,
//      const char *inbuf, size_t inbuflen,
//      char **outbuf, size_t *outbuflen,
//      char **outs, size_t *outslen);
func (mount *MountInfo) mdsCommand(mdsSpec string, args [][]byte, inputBuffer []byte) (buffer []byte, info string, err error) {
	spec := C.CString(mdsSpec)
	defer C.free(unsafe.Pointer(spec))

	argc := len(args)
	argv := make([]*C.char, argc)

	for i, arg := range args {
		argv[i] = C.CString(string(arg))
	}
	// free all array elements in a single defer
	defer func() {
		for i := range argv {
			C.free(unsafe.Pointer(argv[i]))
		}
	}()

	var (
		outs, outbuf       *C.char
		outslen, outbuflen C.size_t
	)
	inbuf := C.CString(string(inputBuffer))
	inbufLen := len(inputBuffer)
	defer C.free(unsafe.Pointer(inbuf))

	ret := C.ceph_mds_command(
		mount.mount,        // cephfs mount ref
		spec,               // mds spec
		&argv[0],           // cmd array
		C.size_t(argc),     // cmd array length
		inbuf,              // bulk input
		C.size_t(inbufLen), // length inbuf
		&outbuf,            // buffer
		&outbuflen,         // buffer length
		&outs,              // status string
		&outslen)

	if outslen > 0 {
		info = C.GoStringN(outs, C.int(outslen))
		C.free(unsafe.Pointer(outs))
	}
	if outbuflen > 0 {
		buffer = C.GoBytes(unsafe.Pointer(outbuf), C.int(outbuflen))
		C.free(unsafe.Pointer(outbuf))
	}
	if ret != 0 {
		return nil, info, getError(ret)
	}

	return buffer, info, nil
}
