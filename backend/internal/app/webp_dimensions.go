package app

import (
	"encoding/binary"
	"fmt"
	"os"
)

// webpDimensions 从 WebP 文件头解析原始像素尺寸，零外部依赖。
// 支持两种容器：
//   - "VP8X"（扩展格式，带 alpha）：画布宽高为 24 位小端，位于扩展块头
//   - "VP8 "（有损）：帧头含 0x9d012a 签名，随后 14 位宽、14 位高
func webpDimensions(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	head := make([]byte, 32)
	if _, err := f.Read(head); err != nil {
		return 0, 0, fmt.Errorf("读取文件头失败: %w", err)
	}
	if len(head) < 30 || string(head[0:4]) != "RIFF" || string(head[8:12]) != "WEBP" {
		return 0, 0, fmt.Errorf("不是 WebP 文件")
	}

	switch string(head[12:16]) {
	case "VP8X":
		w := int(uint32(head[24]) | uint32(head[25])<<8 | uint32(head[26])<<16) + 1
		h := int(uint32(head[27]) | uint32(head[28])<<8 | uint32(head[29])<<16) + 1
		return w, h, nil
	case "VP8 ":
		// 帧头：3 字节 tag + 3 字节同步码 0x9d 0x01 0x2a，随后 2 字节宽、2 字节高
		if head[23] != 0x9d || head[24] != 0x01 || head[25] != 0x2a {
			return 0, 0, fmt.Errorf("VP8 帧头签名不匹配")
		}
		w := int(binary.LittleEndian.Uint16(head[26:28]) & 0x3fff)
		h := int(binary.LittleEndian.Uint16(head[28:30]) & 0x3fff)
		return w, h, nil
	default:
		return 0, 0, fmt.Errorf("不支持的 WebP 容器: %q", string(head[12:16]))
	}
}
