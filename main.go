package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// 自定义 Base64 表（顺序与 Python 版一致）
const variantBase64Table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="

var variantBase64Dict = func() map[int]byte {
	m := make(map[int]byte)
	for i := 0; i < len(variantBase64Table); i++ {
		m[i] = variantBase64Table[i]
	}
	return m
}()

var variantBase64ReverseDict = func() map[byte]int {
	m := make(map[byte]int)
	for i := 0; i < len(variantBase64Table); i++ {
		m[variantBase64Table[i]] = i
	}
	return m
}()

// VariantBase64Encode 自定义 Base64 编码（小端序处理 3 字节块）
func VariantBase64Encode(bs []byte) []byte {
	var result []byte
	blocksCount := len(bs) / 3
	leftBytes := len(bs) % 3
	for i := 0; i < blocksCount; i++ {
		codingInt := int(bs[3*i]) | int(bs[3*i+1])<<8 | int(bs[3*i+2])<<16
		result = append(result, variantBase64Dict[codingInt&0x3f])
		result = append(result, variantBase64Dict[(codingInt>>6)&0x3f])
		result = append(result, variantBase64Dict[(codingInt>>12)&0x3f])
		result = append(result, variantBase64Dict[(codingInt>>18)&0x3f])
	}
	switch leftBytes {
	case 0:
		return result
	case 1:
		codingInt := int(bs[3*blocksCount])
		result = append(result, variantBase64Dict[codingInt&0x3f])
		result = append(result, variantBase64Dict[(codingInt>>6)&0x3f])
		return result
	default: // 2
		codingInt := int(bs[3*blocksCount]) | int(bs[3*blocksCount+1])<<8
		result = append(result, variantBase64Dict[codingInt&0x3f])
		result = append(result, variantBase64Dict[(codingInt>>6)&0x3f])
		result = append(result, variantBase64Dict[(codingInt>>12)&0x3f])
		return result
	}
}

// VariantBase64Decode 自定义 Base64 解码
func VariantBase64Decode(s []byte) ([]byte, error) {
	var result []byte
	blocksCount := len(s) / 4
	leftBytes := len(s) % 4
	for i := 0; i < blocksCount; i++ {
		block := variantBase64ReverseDict[s[4*i]] |
			variantBase64ReverseDict[s[4*i+1]]<<6 |
			variantBase64ReverseDict[s[4*i+2]]<<12 |
			variantBase64ReverseDict[s[4*i+3]]<<18
		result = append(result, byte(block), byte(block>>8), byte(block>>16))
	}
	switch leftBytes {
	case 0:
		return result, nil
	case 2:
		block := variantBase64ReverseDict[s[4*blocksCount]] |
			variantBase64ReverseDict[s[4*blocksCount+1]]<<6
		result = append(result, byte(block))
		return result, nil
	case 3:
		block := variantBase64ReverseDict[s[4*blocksCount]] |
			variantBase64ReverseDict[s[4*blocksCount+1]]<<6 |
			variantBase64ReverseDict[s[4*blocksCount+2]]<<12
		result = append(result, byte(block), byte(block>>8))
		return result, nil
	default:
		return nil, fmt.Errorf("invalid encoding")
	}
}

// EncryptBytes 加密函数：密钥 0x787，逐字节异或并更新 key
func EncryptBytes(key int, bs []byte) []byte {
	result := make([]byte, len(bs))
	for i := 0; i < len(bs); i++ {
		result[i] = bs[i] ^ byte((key>>8)&0xff)
		key = int(result[i])&key | 0x482D
	}
	return result
}

// DecryptBytes 解密函数（对称）
func DecryptBytes(key int, bs []byte) []byte {
	result := make([]byte, len(bs))
	for i := 0; i < len(bs); i++ {
		result[i] = bs[i] ^ byte((key>>8)&0xff)
		key = int(bs[i])&key | 0x482D
	}
	return result
}

// 授权类型枚举
const (
	LicenseProfessional = 1
	LicenseEducational  = 3
	LicensePersonal     = 4
)

// GenerateLicense 生成授权文件（ZIP containing Pro.key），返回生成的文件名
func GenerateLicense(licenseType, count int, userName string, majorVersion, minorVersion int) (string, error) {
	licenseString := fmt.Sprintf("%d#%s|%d%d#%d#%d3%d6%d#%d#%d#%d#",
		licenseType, userName, majorVersion, minorVersion, count,
		majorVersion, minorVersion, minorVersion,
		0, // Unknown
		0, // No Games flag
		0) // No Plugins flag

	encoded := VariantBase64Encode(EncryptBytes(0x787, []byte(licenseString)))
	encodedStr := string(encoded)
	fileName := strings.NewReplacer("/", "", "\\", "").Replace(encodedStr)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// MobaXterm 要求 Pro.key 使用 STORED（无压缩）方式，与 Python zipfile 默认行为一致。
	// 关键：必须预先填好 CRC32 / 大小，否则 Go 会设置 general purpose bit 3（data descriptor），
	// 导致 MobaXterm 26.x 无法解析而激活失败。Python zipfile 默认把 CRC 写在 local header 中。
	modTime, modDate := timeToMSDOS(time.Now())
	hdr := &zip.FileHeader{
		Name:               "Pro.key",
		Method:             zip.Store,
		CRC32:              crc32.ChecksumIEEE([]byte(encodedStr)),
		CompressedSize64:   uint64(len(encodedStr)),
		UncompressedSize64: uint64(len(encodedStr)),
		ModifiedTime:       modTime,
		ModifiedDate:       modDate,
		CreatorVersion:     20,
		ReaderVersion:      20,
		ExternalAttrs:      0x1800000, // 与 Python zipfile 在 Windows 上生成的 external_attrs 一致
	}
	w, err := zw.CreateRaw(hdr)
	if err != nil {
		return "", err
	}
	if _, err := io.WriteString(w, encodedStr); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}

	if err := os.WriteFile(fileName, buf.Bytes(), 0644); err != nil {
		return "", err
	}
	return fileName, nil
}

// timeToMSDOS 将 time.Time 转换为 ZIP MS-DOS 格式的时间/日期
func timeToMSDOS(t time.Time) (uint16, uint16) {
	t = t.UTC()
	timeVal := uint16(t.Second()/2) |
		uint16(t.Minute())<<5 |
		uint16(t.Hour())<<11
	dateVal := uint16(t.Day()) |
		uint16(int(t.Month()))<<5 |
		uint16(t.Year()-1980)<<9
	return timeVal, dateVal
}

// get_lc 从查询参数解析并生成授权
func getLicense(r *http.Request) (string, error) {
	name := r.URL.Query().Get("name")
	version := r.URL.Query().Get("ver")
	countStr := r.URL.Query().Get("count")
	if countStr == "" {
		countStr = "1"
	}
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 0 {
		return "", fmt.Errorf("invalid count")
	}

	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid version")
	}
	majorVersion, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", fmt.Errorf("invalid major version")
	}
	minorVersion, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid minor version")
	}

	return GenerateLicense(LicenseProfessional, count, name, majorVersion, minorVersion)
}

// index.html 内容
const indexHTML = `<!DOCTYPE html>
<html lang="zh">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>MobaXterm KeyGen</title>
    <style>
        :root {
            --bg: #0c1116;
            --panel: #141b22;
            --line: #263545;
            --text: #dfe9f3;
            --muted: #8fa1b6;
            --accent: #3ddc97;
            --accent-dim: #1f7a57;
            --danger: #ff6b6b;
        }
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            background:
                radial-gradient(1200px 600px at 50% -10%, #16202b 0%, transparent 60%),
                var(--bg);
            color: var(--text);
            font-family: "SF Mono", "JetBrains Mono", ui-monospace, "Cascadia Code", Menlo, Consolas, monospace;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 24px;
        }
        .window {
            width: 100%;
            max-width: 480px;
            background: var(--panel);
            border: 1px solid var(--line);
            border-radius: 14px;
            overflow: hidden;
            box-shadow: 0 24px 70px rgba(0,0,0,.5);
        }
        .titlebar {
            display: flex;
            align-items: center;
            gap: 8px;
            padding: 12px 16px;
            background: linear-gradient(180deg, #1a232e, #141b22);
            border-bottom: 1px solid var(--line);
        }
        .dot { width: 11px; height: 11px; border-radius: 50%; }
        .dot.r { background: #ff5f56; }
        .dot.y { background: #ffbd2e; }
        .dot.g { background: #27c93f; }
        .titlebar .path {
            margin-left: 8px;
            color: #60748a;
            font-size: 12px;
            letter-spacing: .4px;
        }
        .body { padding: 30px 30px 32px; }
        .header { margin-bottom: 24px; }
        .header h1 {
            font-size: 22px;
            font-weight: 700;
            letter-spacing: -.3px;
            color: var(--text);
        }
        .header h1 span { color: var(--accent); }
        .header p {
            margin-top: 6px;
            font-size: 13px;
            color: var(--muted);
            line-height: 1.55;
        }
        .header code {
            display: inline-block;
            padding: 2px 6px;
            background: rgba(61,220,151,.12);
            color: var(--accent);
            border-radius: 5px;
            font-size: 11px;
            font-weight: 600;
        }
        label {
            display: flex;
            align-items: baseline;
            gap: 8px;
            font-size: 11px;
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 1.2px;
            color: #a3b6cc;
            margin: 18px 0 8px;
        }
        label span { color: #60748a; font-weight: 400; }
        .field {
            display: flex;
            align-items: center;
            background: #0e151c;
            border: 1px solid var(--line);
            border-radius: 9px;
            padding: 0 14px;
            transition: border-color .15s, box-shadow .15s, background .15s;
        }
        .field:hover { background: #0a1118; }
        .field:focus-within {
            border-color: var(--accent-dim);
            background: #090f15;
            box-shadow: 0 0 0 3px rgba(61,220,151,.12);
        }
        .field .prompt {
            color: var(--accent-dim);
            font-size: 13px;
            font-weight: 600;
            user-select: none;
            margin-right: 10px;
        }
        input {
            flex: 1;
            background: transparent;
            border: none;
            outline: none;
            color: var(--text);
            font-family: inherit;
            font-size: 14.5px;
            font-weight: 500;
            letter-spacing: .5px;
            padding: 13px 0;
        }
        input::placeholder { color: #405363; }
        button {
            width: 100%;
            margin-top: 28px;
            padding: 14px;
            background: linear-gradient(180deg, var(--accent), var(--accent-dim));
            color: #04130c;
            font-family: inherit;
            font-size: 14px;
            font-weight: 800;
            letter-spacing: 1px;
            border: none;
            border-radius: 9px;
            cursor: pointer;
            display: inline-flex;
            align-items: center;
            justify-content: center;
            gap: 8px;
            transition: transform .08s, filter .15s, box-shadow .15s;
            box-shadow: 0 6px 18px rgba(61,220,151,.22);
        }
        button:hover { filter: brightness(1.08); }
        button:active { transform: translateY(1px); }
        button .arrow { font-size: 16px; line-height: 1; }
        .hint {
            margin-top: 18px;
            padding: 10px 12px;
            background: rgba(143,161,182,.08);
            border-left: 2px solid var(--accent-dim);
            border-radius: 0 6px 6px 0;
            font-size: 11px;
            color: var(--muted);
            line-height: 1.6;
        }
        .hint::before { content: ">"; color: var(--accent-dim); margin-right: 8px; }
        .hint code {
            color: var(--text);
            background: rgba(143,161,182,.12);
            padding: 1px 4px;
            border-radius: 4px;
        }
        .steps {
            margin-top: 20px;
            padding: 16px 18px;
            background: rgba(61,220,151,.06);
            border: 1px solid rgba(61,220,151,.18);
            border-radius: 9px;
        }
        .steps h2 {
            font-size: 12px;
            font-weight: 700;
            text-transform: uppercase;
            letter-spacing: 1px;
            color: var(--accent);
            margin-bottom: 10px;
        }
        .steps ol { margin: 0; padding-left: 18px; }
        .steps li {
            font-size: 12px;
            color: var(--muted);
            line-height: 1.9;
        }
        .steps li b { color: var(--text); font-weight: 600; }
        .steps code {
            color: var(--accent);
            background: rgba(61,220,151,.12);
            padding: 1px 5px;
            border-radius: 4px;
            font-size: 11px;
        }
    </style>
</head>
<body>
    <div class="window">
        <div class="titlebar">
            <span class="dot r"></span><span class="dot y"></span><span class="dot g"></span>
            <span class="path">~/mobaxterm-keygen — generator</span>
        </div>
        <div class="body">
            <div class="header">
                <h1>MobaXterm <span>KeyGen</span></h1>
                <p>生成 <code>.mxtpro</code> 授权文件。填写下方参数，点击按钮后浏览器将自动下载。</p>
            </div>
            <form action="/gen" method="get">
                <label>用户名 <span>/ Username</span></label>
                <div class="field"><span class="prompt">$</span><input type="text" name="name" value="admin" autocomplete="off"></div>

                <label>版本号 <span>/ Version</span></label>
                <div class="field"><span class="prompt">v</span><input type="text" name="ver" value="26.3" placeholder="21.0" autocomplete="off"></div>

                <label>授权数量 <span>/ Count</span></label>
                <div class="field"><span class="prompt">#</span><input type="text" name="count" value="1" autocomplete="off"></div>

                <button type="submit"><span class="arrow">→</span>生成并下载</button>
            </form>
            <p class="hint">直接通过 URL 传参：<code>/gen?name=admin&amp;ver=26.3&amp;count=1</code></p>
            <div class="steps">
                <h2>激活步骤</h2>
                <ol>
                    <li>点击 <b>生成并下载</b>，浏览器会保存 <code>Custom.mxtpro</code> 文件。</li>
                    <li>将 <code>Custom.mxtpro</code> 放到 MobaXterm 安装目录（与 <code>MobaXterm.exe</code> 同级）。</li>
                    <li>重新启动 MobaXterm，在菜单 <b>Help → Register</b> 中即可看到已激活的专业版授权。</li>
                    <li>若未生效，请以管理员身份运行一次 MobaXterm 再重启。</li>
                </ol>
            </div>
        </div>
    </div>
</body>
</html>`

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, indexHTML)
	})

	http.HandleFunc("/gen", func(w http.ResponseWriter, r *http.Request) {
		lc, err := getLicense(r)
		if err != nil || len(lc) <= 5 || !fileExists(lc) {
			http.Error(w, "请检查用户名版本号是否正确！", http.StatusBadRequest)
			return
		}
		defer os.Remove(lc) // 下载后清理临时文件
		w.Header().Set("Content-Disposition", `attachment; filename="Custom.mxtpro"`)
		w.Header().Set("Content-Type", "application/zip")
		data, err := os.ReadFile(lc)
		if err != nil {
			http.Error(w, "读取文件失败", http.StatusInternalServerError)
			return
		}
		w.Write(data)
	})

	const addr = "0.0.0.0:50000"
	fmt.Printf("Running on http://localhost:50000\n")

	go func() {
		time.Sleep(400 * time.Millisecond)
		openBrowser("http://localhost:50000")
	}()

	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Printf("server error: %v\n", err)
	}
}

// openBrowser 跨平台打开默认浏览器
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", ""}
	case "darwin":
		cmd = "open"
	default: // linux 等
		cmd = "xdg-open"
	}
	args = append(args, url)
	if err := exec.Command(cmd, args...).Start(); err != nil {
		fmt.Printf("无法自动打开浏览器，请手动访问 %s\n", url)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
