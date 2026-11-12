//go:build ignore

// Generates rsrc_windows_amd64.syso, which embeds the app icon into
// EngTreeViewer.exe (file, shortcut and taskbar icon). "go build" picks the
// .syso up automatically when building for Windows 64-bit.
//
// Only needed again if the icon changes:
//
//	go run tools/makeicon/main.go assets/app_icon.ico
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

type icoImage struct {
	width, height, colors byte
	planes, bitCount      uint16
	data                  []byte
}

func readICO(path string) ([]icoImage, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 6 || binary.LittleEndian.Uint16(raw[2:]) != 1 {
		return nil, fmt.Errorf("%s is not a valid .ico file", path)
	}
	n := int(binary.LittleEndian.Uint16(raw[4:]))
	var imgs []icoImage
	for i := 0; i < n; i++ {
		e := raw[6+16*i:]
		size := binary.LittleEndian.Uint32(e[8:])
		off := binary.LittleEndian.Uint32(e[12:])
		if int(off+size) > len(raw) {
			return nil, fmt.Errorf("image %d is out of the file bounds", i)
		}
		imgs = append(imgs, icoImage{
			width: e[0], height: e[1], colors: e[2],
			planes: binary.LittleEndian.Uint16(e[4:]), bitCount: binary.LittleEndian.Uint16(e[6:]),
			data: raw[off : off+size],
		})
	}
	return imgs, nil
}

func align(b *bytes.Buffer, n int) {
	for b.Len()%n != 0 {
		b.WriteByte(0)
	}
}

func main() {
	icoPath := "assets/app_icon.ico"
	if len(os.Args) > 1 {
		icoPath = os.Args[1]
	}
	imgs, err := readICO(icoPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	const (
		rtIcon      = 3
		rtGroupIcon = 14
		lang        = 0x0409 // en-US
		dirSize     = 16
		entrySize   = 8
		dataEntSize = 16
	)
	n := len(imgs)

	// .rsrc section layout:
	// raiz (2 tipos) | dir RT_ICON (n ids) | dir RT_GROUP_ICON (1 id)
	// | n language dirs (icons) | 1 language dir (group) | n+1 data entries | data
	rootOff := 0
	iconDirOff := rootOff + dirSize + 2*entrySize
	groupDirOff := iconDirOff + dirSize + n*entrySize
	langOff := groupDirOff + dirSize + entrySize
	langDirSize := dirSize + entrySize
	dataEntOff := langOff + (n+1)*langDirSize
	dataOff := dataEntOff + (n+1)*dataEntSize

	var group bytes.Buffer
	binary.Write(&group, binary.LittleEndian, []uint16{0, 1, uint16(n)})
	for i, im := range imgs {
		group.Write([]byte{im.width, im.height, im.colors, 0})
		binary.Write(&group, binary.LittleEndian, im.planes)
		binary.Write(&group, binary.LittleEndian, im.bitCount)
		binary.Write(&group, binary.LittleEndian, uint32(len(im.data)))
		binary.Write(&group, binary.LittleEndian, uint16(i+1))
	}
	blobs := make([][]byte, 0, n+1)
	for _, im := range imgs {
		blobs = append(blobs, im.data)
	}
	blobs = append(blobs, group.Bytes())

	var sec bytes.Buffer
	u16 := func(v uint16) { binary.Write(&sec, binary.LittleEndian, v) }
	u32 := func(v uint32) { binary.Write(&sec, binary.LittleEndian, v) }
	dirHeader := func(ids int) {
		u32(0)
		u32(0)
		u16(0)
		u16(0)
		u16(0)
		u16(uint16(ids))
	}
	const sub = 0x80000000

	// root
	dirHeader(2)
	u32(rtIcon)
	u32(sub | uint32(iconDirOff))
	u32(rtGroupIcon)
	u32(sub | uint32(groupDirOff))
	// RT_ICON
	dirHeader(n)
	for i := 0; i < n; i++ {
		u32(uint32(i + 1))
		u32(sub | uint32(langOff+i*langDirSize))
	}
	// RT_GROUP_ICON
	dirHeader(1)
	u32(1)
	u32(sub | uint32(langOff+n*langDirSize))
	// language directories
	for i := 0; i <= n; i++ {
		dirHeader(1)
		u32(lang)
		u32(uint32(dataEntOff + i*dataEntSize))
	}
	// data entries (OffsetToData is fixed up by the linker via relocation)
	var relocs []uint32
	off := dataOff
	offsets := make([]int, len(blobs))
	for i, b := range blobs {
		off = (off + 7) &^ 7
		offsets[i] = off
		off += len(b)
	}
	for i, b := range blobs {
		relocs = append(relocs, uint32(sec.Len()))
		u32(uint32(offsets[i]))
		u32(uint32(len(b)))
		u32(0)
		u32(0)
	}
	if sec.Len() != dataOff {
		panic("layout inconsistente")
	}
	for i, b := range blobs {
		for sec.Len() < offsets[i] {
			sec.WriteByte(0)
		}
		sec.Write(b)
	}
	align(&sec, 8)

	// COFF object (AMD64) with one .rsrc section
	const (
		fileHdr  = 20
		secHdr   = 40
		relocSz  = 10
		symSz    = 18
		amd64    = 0x8664
		addr32nb = 3
	)
	rawPtr := fileHdr + secHdr
	relocPtr := rawPtr + sec.Len()
	symPtr := relocPtr + len(relocs)*relocSz

	var obj bytes.Buffer
	w := func(v any) { binary.Write(&obj, binary.LittleEndian, v) }
	w(uint16(amd64))
	w(uint16(1))
	w(uint32(0))
	w(uint32(symPtr))
	w(uint32(1))
	w(uint16(0))
	w(uint16(0))
	obj.Write([]byte(".rsrc\x00\x00\x00"))
	w(uint32(0))
	w(uint32(0))
	w(uint32(sec.Len()))
	w(uint32(rawPtr))
	w(uint32(relocPtr))
	w(uint32(0))
	w(uint16(len(relocs)))
	w(uint16(0))
	w(uint32(0x40000040)) // initialized data, read
	obj.Write(sec.Bytes())
	for _, r := range relocs {
		w(r)
		w(uint32(0)) // symbol 0 = start of .rsrc section
		w(uint16(addr32nb))
	}
	obj.Write([]byte(".rsrc\x00\x00\x00"))
	w(uint32(0))
	w(int16(1))
	w(uint16(0))
	w(uint8(3)) // IMAGE_SYM_CLASS_STATIC
	w(uint8(0))
	w(uint32(4)) // empty string table

	if err := os.WriteFile("rsrc_windows_amd64.syso", obj.Bytes(), 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("rsrc_windows_amd64.syso generated with %d icon sizes.\n", n)
}
