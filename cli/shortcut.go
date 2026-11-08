package cli

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"unicode/utf16"
)

// shortcutFile is a Shell Link (.lnk) to target carrying appID as its
// System.AppUserModel.ID, laid out as [MS-SHLLINK] has it. Windows shows
// toasts sent from appID under the name of the Start-menu shortcut that
// carries it, so this is what makes them show as Code Central. The link
// names its target by path (LinkInfo) rather than by shell item IDs,
// which Windows resolves the same way.
func shortcutFile(target, appID string) []byte {
	var b bytes.Buffer
	le := func(v any) { binary.Write(&b, binary.LittleEndian, v) }

	// ShellLinkHeader: HasLinkInfo, HasName, HasWorkingDir, IsUnicode.
	le(uint32(0x4c))
	b.Write([]byte{0x01, 0x14, 0x02, 0, 0, 0, 0, 0, 0xc0, 0, 0, 0, 0, 0, 0, 0x46}) // CLSID_ShellLink
	le(uint32(0x2 | 0x4 | 0x10 | 0x80))
	le(uint32(0))          // FileAttributes
	le([3]uint64{})        // creation, access and write times
	le([3]uint32{0, 0, 1}) // FileSize, IconIndex, ShowCommand (SW_SHOWNORMAL)
	le([2]uint16{})        // HotKey, Reserved1
	le([2]uint32{})        // Reserved2, Reserved3

	// LinkInfo: a local path, in ANSI and in Unicode (a 0x24 header).
	volumeID := []byte{17, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 0x10, 0, 0, 0, 0} // DRIVE_FIXED, no serial, no label
	ansi := append(ansiPath(target), 0)
	unicode := utf16z(target)
	const header = 0x24
	volumeAt := header
	ansiAt := volumeAt + len(volumeID)
	suffixAt := ansiAt + len(ansi)
	unicodeAt := suffixAt + 1
	unicodeSuffixAt := unicodeAt + len(unicode)
	le([]uint32{uint32(unicodeSuffixAt + 2), header, 0x1, // VolumeIDAndLocalBasePath
		uint32(volumeAt), uint32(ansiAt), 0, uint32(suffixAt), uint32(unicodeAt), uint32(unicodeSuffixAt)})
	b.Write(volumeID)
	b.Write(ansi)
	b.WriteByte(0) // CommonPathSuffix
	b.Write(unicode)
	b.Write([]byte{0, 0}) // CommonPathSuffixUnicode

	// StringData: the description, then the working directory.
	for _, s := range []string{"Code Central's Notifications", filepath.Dir(target)} {
		u := utf16.Encode([]rune(s))
		le(uint16(len(u)))
		le(u)
	}

	// A PropertyStoreDataBlock holding one serialized property storage:
	// the System.AppUserModel property set's ID (5), a VT_LPWSTR.
	id := utf16z(appID)
	for len(id)%4 != 0 {
		id = append(id, 0)
	}
	value := 4 + 4 + 1 + 2 + 2 + 4 + len(id)
	storage := 4 + 4 + 16 + value + 4
	le([]uint32{uint32(8 + storage + 4), 0xa0000009, uint32(storage), 0x53505331})
	b.Write([]byte{0x55, 0x28, 0x4c, 0x9f, 0x79, 0x9f, 0x39, 0x4b, 0xa8, 0xd0, 0xe1, 0xd4, 0x2d, 0xe1, 0xd5, 0xf3}) // PKEY_AppUserModel_ID's set
	le([]uint32{uint32(value), 5})
	b.WriteByte(0)
	le([]uint16{0x1f, 0})
	le(uint32(len(utf16.Encode([]rune(appID))) + 1))
	b.Write(id)
	le(uint32(0)) // no more values
	le(uint32(0)) // no more storages

	le(uint32(0)) // TerminalBlock
	return b.Bytes()
}

// utf16z is s as NUL-terminated UTF-16LE.
func utf16z(s string) []byte {
	u := utf16.Encode([]rune(s + "\x00"))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return b
}

// ansiPath is p in the LinkInfo's ANSI field, which Windows reads only
// when there is no Unicode path; anything beyond ASCII becomes '?'.
func ansiPath(p string) []byte {
	var b []byte
	for _, r := range p {
		if r > 0x7f {
			r = '?'
		}
		b = append(b, byte(r))
	}
	return b
}
