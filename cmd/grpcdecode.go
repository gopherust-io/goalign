package cmd

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/gopherust-io/goalign/internal/bytesconv"
	"github.com/spf13/cobra"
)

var grpcDecodeCmd = &cobra.Command{
	Use:   "grpc-decode [hex-file]",
	Short: "Decode gRPC/protobuf hex dump to JSON",
	Long: `Decode gRPC/protobuf binary data from hex dump to JSON format.
Can read from file or stdin.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var hexData string
		var err error

		if len(args) == 0 {
			// Read from stdin
			data, err := os.ReadFile("/dev/stdin")
			if err != nil {
				return fmt.Errorf("failed to read stdin: %w", err)
			}
			hexData = strings.TrimSpace(bytesconv.BytesToString(data))
		} else {
			// Read from file
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}
			hexData = strings.TrimSpace(bytesconv.BytesToString(data))
		}

		// Clean hex data (remove spaces, newlines, offsets, ASCII column)
		hexData = cleanHexData(hexData)

		// Decode hex to bytes
		bytes, err := hex.DecodeString(hexData)
		if err != nil {
			return fmt.Errorf("failed to decode hex: %w", err)
		}

		messageBytes, err := messageBytesFromFrame(bytes)
		if err != nil {
			return err
		}

		// Decode protobuf wire format
		result := decodeProtobufWireFormat(messageBytes)

		// Output as JSON
		jsonOutput, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}

		fmt.Println(bytesconv.BytesToString(jsonOutput))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(grpcDecodeCmd)
}

// messageBytesFromFrame strips an uncompressed gRPC frame header when present.
// Compressed frames are rejected only when the 5-byte header looks like a real
// gRPC frame; otherwise the payload is treated as raw protobuf.
func messageBytesFromFrame(bytes []byte) ([]byte, error) {
	if len(bytes) < 5 {
		return bytes, nil
	}
	compressionFlag := bytes[0]
	messageLength := int(bytes[1])<<24 | int(bytes[2])<<16 | int(bytes[3])<<8 | int(bytes[4])
	validFrame := messageLength > 0 && messageLength < 1000000 && 5+messageLength <= len(bytes)
	switch {
	case compressionFlag == 1 && validFrame:
		return nil, fmt.Errorf("compressed gRPC frames are not supported (flag=0x%02x)", compressionFlag)
	case compressionFlag == 0 && validFrame:
		return bytes[5 : 5+messageLength], nil
	default:
		return bytes, nil
	}
}

func cleanHexData(hexData string) string {
	// Remove offset column (hex addresses at start of lines)
	lines := strings.Split(hexData, "\n")
	var cleaned []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if bytesconv.IsEmpty(line) {
			continue
		}

		// Skip lines that are just offsets or separators
		if strings.HasPrefix(line, "Offset") || strings.HasPrefix(line, "---") {
			continue
		}

		// Extract hex values (format: OFFSET: hex hex hex ...)
		parts := strings.Split(line, ":")
		if len(parts) > 1 {
			line = strings.Join(parts[1:], ":")
		}

		// Remove ASCII column (everything after tab or after 48 chars of hex)
		// Split by tab first
		tabParts := strings.Split(line, "\t")
		if len(tabParts) > 0 {
			line = tabParts[0]
		}

		// Extract only hex pairs (00-FF)
		var hexPairs []string
		words := strings.Fields(line)
		for _, word := range words {
			word = strings.TrimSpace(word)
			if len(word) == 2 {
				// Check if it's valid hex
				if isHex(word) {
					hexPairs = append(hexPairs, word)
				}
			}
		}

		if len(hexPairs) > 0 {
			cleaned = append(cleaned, strings.Join(hexPairs, ""))
		}
	}

	return strings.Join(cleaned, "")
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func decodeProtobufWireFormat(data []byte) map[string]interface{} {
	result := make(map[string]interface{})
	result["message"] = parseProtobufFields(data)
	result["metadata"] = extractMetadata(data)
	result["readable_strings"] = extractStrings(data)
	return result
}

func parseProtobufFields(data []byte) []map[string]interface{} {
	var fields []map[string]interface{}
	pos := 0
	maxIterations := 1000 // Prevent infinite loops
	iteration := 0

	for pos < len(data) && iteration < maxIterations {
		iteration++
		if pos+1 > len(data) {
			break
		}

		// Read tag as a protobuf varint (field number + wire type).
		tag, newPos := decodeVarint(data, pos)
		if newPos == pos {
			break
		}
		pos = newPos
		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x07)

		if fieldNum == 0 {
			// Field number 0 is invalid, might be padding or end of message
			break
		}

		field := make(map[string]interface{})
		field["field_number"] = fieldNum
		field["wire_type"] = wireType
		field["wire_type_name"] = getWireTypeName(wireType)

		switch wireType {
		case 0: // Varint
			if pos < len(data) {
				val, newPos := decodeVarint(data, pos)
				field["value"] = val
				pos = newPos
			}
		case 1: // 64-bit
			if pos+8 <= len(data) {
				field["value"] = hex.EncodeToString(data[pos : pos+8])
				field["value_type"] = "fixed64"
				pos += 8
			} else {
				goto end
			}
		case 2: // Length-delimited (string, bytes, embedded message)
			if pos < len(data) {
				length, newPos := decodeVarint(data, pos)
				pos = newPos
				if length == 0 {
					field["value"] = ""
					field["value_type"] = "string"
					fields = append(fields, field)
					continue
				}
				if length > uint64(math.MaxInt) || pos > len(data) || int(length) > len(data)-pos {
					goto end
				}
				bytes := data[pos : pos+int(length)]

				// Heuristic: check if this looks like a nested message
				// A message should start with a valid field tag (field_num > 0, wire_type valid)
				fieldNum := int(bytes[0]) >> 3
				wireType := int(bytes[0]) & 0x07
				isLikelyMessage := len(bytes) > 1 &&
					wireType <= 5 && // Valid wire type
					fieldNum > 0 && // Valid field number
					fieldNum < 536870912 // Reasonable field number (2^29-1)

				var nested []map[string]interface{}
				if isLikelyMessage {
					nested = parseProtobufFields(bytes)
					// Only use as nested message if we got reasonable results
					if len(nested) > 0 && len(nested) < 100 && looksLikeValidProtobuf(nested) {
						field["nested_message"] = nested
						field["value_type"] = "message"
						field["raw_hex"] = hex.EncodeToString(bytes)
					} else {
						// Failed as message, treat as string/bytes
						isLikelyMessage = false
					}
				}

				if !isLikelyMessage || len(nested) == 0 {
					// Not a nested message, try to decode as string
					if utf8.Valid(bytes) || isPrintable(bytes) {
						field["value"] = bytesconv.BytesToString(bytes)
						field["value_type"] = "string"
						field["hex"] = hex.EncodeToString(bytes)
					} else {
						field["value"] = hex.EncodeToString(bytes)
						field["value_type"] = "bytes"
					}
				}

				pos += int(length)
			} else {
				goto end
			}
		case 5: // 32-bit
			if pos+4 <= len(data) {
				field["value"] = hex.EncodeToString(data[pos : pos+4])
				field["value_type"] = "fixed32"
				pos += 4
			} else {
				goto end
			}
		default:
			field["error"] = "unsupported wire type"
			goto end
		}

		fields = append(fields, field)
	}

end:
	return fields
}

func decodeVarint(data []byte, pos int) (uint64, int) {
	var result uint64
	var shift uint
	currentPos := pos

	for currentPos < len(data) {
		b := data[currentPos]
		result |= uint64(b&0x7F) << shift
		currentPos++
		if b&0x80 == 0 {
			break
		}
		shift += 7
		if shift >= 64 {
			break
		}
	}

	return result, currentPos
}

func getWireTypeName(wireType int) string {
	switch wireType {
	case 0:
		return "varint"
	case 1:
		return "fixed64"
	case 2:
		return "length_delimited"
	case 5:
		return "fixed32"
	default:
		return "unknown"
	}
}

func isPrintable(data []byte) bool {
	for _, b := range data {
		if b < 32 && b != 9 && b != 10 && b != 13 {
			return false
		}
		if b > 126 {
			return false
		}
	}
	return true
}

func looksLikeValidProtobuf(fields []map[string]interface{}) bool {
	// Check if fields have reasonable values
	// Too many fields with very large varint values suggests we're parsing random data
	largeVarintCount := 0
	for _, field := range fields {
		if wireType, ok := field["wire_type"].(int); ok && wireType == 0 {
			if val, ok := field["value"].(uint64); ok && val > 1000000000000 {
				largeVarintCount++
			}
		}
	}
	// If more than 30% are suspiciously large, it's probably not valid protobuf
	return largeVarintCount < len(fields)/3
}

func extractMetadata(data []byte) map[string]interface{} {
	metadata := make(map[string]interface{})
	metadata["total_length"] = len(data)
	metadata["hex_preview"] = hex.EncodeToString(data[:min(64, len(data))])

	// Check for gRPC status
	if idx := bytes.Index(data, bytesconv.StringToBytes("grpc-status")); idx >= 0 {
		metadata["grpc_status_found"] = true
		if idx+20 < len(data) {
			end := min(idx+50, len(data))
			metadata["grpc_status_section"] = bytesconv.BytesToString(data[idx:end])
		}
	}

	return metadata
}

func extractStrings(bytes []byte) []string {
	var strings []string
	var current []byte

	for i := 0; i < len(bytes); i++ {
		if bytes[i] >= 32 && bytes[i] < 127 {
			current = append(current, bytes[i])
		} else {
			if len(current) >= 4 {
				strings = append(strings, bytesconv.BytesToString(current))
			}
			current = nil
		}
	}
	if len(current) >= 4 {
		strings = append(strings, bytesconv.BytesToString(current))
	}

	return strings
}
