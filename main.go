package main

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/ncruces/go-sqlite3/vfs/memdb"
)

var FOLDER_TAR string

type Data struct {
	Nama             string `json:"name"`
	Number           int    `json:"number"`
	RefreshToken     string `json:"refresh_token"`
	SubscriberId     string `json:"subscriber_id"`
	SubscriptionType string `json:"subscription_type"`
}

func FindInTarGz(tarGzPath string, pattern string) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	var ck []Data

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		base := filepath.Base(hdr.Name)

		if base == "ciam_ultimate" {
			fmt.Printf("📦 %s → %s (%d bytes)\n", tarGzPath, hdr.Name, hdr.Size)

			data := make([]byte, hdr.Size)
			if _, err := io.ReadFull(tr, data); err != nil {
				return err
			}

			// 2) register ke memdb VFS
			memName := "ciam_ultimate.db"
			memdb.Create(memName, data)
			defer memdb.Delete(memName) // DB in-memory dibersihkan setelah selesai

			// 3) buka dengan database/sql menggunakan VFS memdb
			dsn := "file:/" + memName + "?vfs=memdb"
			db, err := sql.Open("sqlite3", dsn)
			if err != nil {
				return err
			}
			defer db.Close()

			profiles, _ := readTable(db, "ciam_profile")
			sessions, _ := readTable(db, "ciam_session")

			for _, p := range profiles {
				name := toString(p["name"])
				nomer := toString(p["msisdn"])

				num, _ := strconv.Atoi(nomer)

				subscription_type := toString(p["subscription_type"])
				subscriber_id := toString(p["subscriber_id"])
				if name == "" {
					name = "Kosong"
				}

				for _, c := range sessions {
					// match msisdn
					if nomer == toString(c["msisdn"]) {
						refreshToken := toString(c["refresh_token"])

						ck = append(ck, Data{
							Nama:             Capitalize(name),
							Number:           num,
							RefreshToken:     refreshToken,
							SubscriptionType: subscription_type,
							SubscriberId:     subscriber_id,
						})
					}
				}
			}
		}
	}

	jsonData, err := json.MarshalIndent(ck, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(jsonData))
	if err := os.WriteFile(fmt.Sprintf("%s/refresh-tokens.json", FOLDER_TAR), jsonData, 0644); err != nil {
		return err
	}

	return nil
}

func toString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(t)
	}
}

func Capitalize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])

	for i := 1; i < len(runes); i++ {
		runes[i] = unicode.ToLower(runes[i])
	}
	return string(runes)
}

func readTable(db *sql.DB, table string) ([]map[string]any, error) {
	query := fmt.Sprintf("SELECT * FROM %s", table)

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]any

	for rows.Next() {
		// siapkan penampung nilai
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}

		// scan satu baris
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}

		// buat map kolom -> nilai
		rowMap := make(map[string]any)
		for i, col := range cols {
			val := vals[i]

			// banyak driver SQLite kembalikan TEXT/BLOB sebagai []byte
			if b, ok := val.([]byte); ok {
				val = string(b)
			}

			rowMap[col] = val
		}

		result = append(result, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func FolderExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func main() {

	FOLDER_TAR = "/sdcard/tar-xl"
	if runtime.GOOS == "windows" {
		FOLDER_TAR = "tar-xl"
	}

	if !FolderExists(FOLDER_TAR) {
		err := os.MkdirAll(FOLDER_TAR, 0755)
		if err != nil {
			panic(err)
		}
		fmt.Println("📁 Folder dibuat:", FOLDER_TAR)
	}

	files, err := filepath.Glob(fmt.Sprintf("%s/*.tar.gz", FOLDER_TAR))
	if err != nil {
		panic(err)
	}

	if len(files) == 0 {
		fmt.Printf("Tidak ada tar.gz, copy file tar.gz to %s\n", FOLDER_TAR)
		return
	}

	for _, file := range files {
		fmt.Println("🧩 Scan:", file)
		if err := FindInTarGz(file, "*ciam_ultimate*"); err != nil {
			fmt.Println("Error:", err)
		}
	}
}
