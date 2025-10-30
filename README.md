# GoAlign

A CLI tool for analyzing Go struct alignment issues, similar to [aligo](https://github.com/essentialkaos/aligo).

## Features

- 🔍 **Struct Analysis**: Detects struct alignment issues that waste memory
- 📊 **Multiple Output Formats**: Text, JSON, and table formats
- 🚫 **Ignore Comments**: Support for `// goalign:ignore` comments
- 📁 **Recursive Analysis**: Analyze entire directories
- 🎯 **Severity Levels**: Categorizes issues by severity (high/medium/low)
- 📈 **Detailed Reports**: Shows field sizes, offsets, and alignment info

## Installation

```bash
go install github.com/nekruzjm/goalign@latest
```

## Usage

### Basic Analysis

```bash
# Analyze current directory
goalign analyze

# Analyze specific file
goalign analyze main.go

# Analyze directory recursively
goalign analyze -r ./src

# Exclude certain patterns
goalign analyze -r -e vendor/,test/ ./src
```

### Output Formats

```bash
# Text format (default)
goalign analyze -f text

# JSON format
goalign analyze -f json

# Table format
goalign analyze -f table
```

### Verbose Output

```bash
goalign analyze -v
```

## Example

Given this struct with alignment issues:

```go
type BadStruct struct {
    A bool    // 1 byte
    B int64   // 8 bytes (7 bytes padding before)
    C int32   // 4 bytes
    D bool    // 1 byte (3 bytes padding before)
}
```

GoAlign will report:

```
📁 example.go
================

🟡 BadStruct (line 1)
   Struct 'BadStruct' has 10 bytes of padding (62% waste)
   Fields:
     A bool (size: 1, offset: 0, align: 1)
     B int64 (size: 8, offset: 8, align: 8)
     C int32 (size: 4, offset: 16, align: 4)
     D bool (size: 1, offset: 20, align: 1)

📊 Summary: 1 issues found, 10 bytes wasted
```

## Ignoring Structs

Add a comment to ignore specific structs:

```go
// This struct is intentionally misaligned for compatibility
// goalign:ignore
type LegacyStruct struct {
    A bool
    B int64
}
```

## GitHub Actions Integration

Create `.github/workflows/goalign.yml`:

```yaml
name: GoAlign
on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main ]

jobs:
  goalign:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v3
      with:
        go-version: '1.21'
    - name: Install GoAlign
      run: go install github.com/nekruzjm/goalign@latest
    - name: Run GoAlign
      run: goalign analyze -r .
```

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
