package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// Đọc file credentials.json
func readCredentials() *oauth2.Config {
	data, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatal("❌ Không tìm thấy file credentials.json")
	}

	config, err := google.ConfigFromJSON(data, drive.DriveReadonlyScope)
	if err != nil {
		log.Fatal("❌ File credentials.json không hợp lệ")
	}

	return config
}

// Lấy token (xác thực lần đầu)
func getToken(config *oauth2.Config) *oauth2.Token {
	// Thử đọc token đã lưu
	tokenFile := "token.json"
	data, err := os.ReadFile(tokenFile)
	if err == nil {
		var token oauth2.Token
		json.Unmarshal(data, &token)
		return &token
	}

	// Chưa có token -> yêu cầu xác thực
	config.RedirectURL = "urn:ietf:wg:oauth:2.0:oob"
	url := config.AuthCodeURL("state", oauth2.AccessTypeOffline)

	fmt.Println("\n=== XÁC THỰC LẦN ĐẦU ===")
	fmt.Println("1. Mở link này trong trình duyệt:")
	fmt.Println(url)
	fmt.Println("\n2. Đăng nhập và cho phép quyền truy cập")
	fmt.Print("3. Nhập mã xác thực: ")

	var code string
	fmt.Scan(&code)

	token, err := config.Exchange(context.Background(), code)
	if err != nil {
		log.Fatal("❌ Mã xác thực không đúng")
	}

	// Lưu token
	data, _ = json.Marshal(token)
	os.WriteFile(tokenFile, data, 0600)
	fmt.Println("✅ Xác thực thành công!\n")

	return token
}

// Tính dung lượng thư mục (KB, MB, GB)
func formatSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	kb := float64(bytes) / 1024
	if kb < 1024 {
		return fmt.Sprintf("%.2f KB", kb)
	}
	mb := kb / 1024
	if mb < 1024 {
		return fmt.Sprintf("%.2f MB", mb)
	}
	gb := mb / 1024
	return fmt.Sprintf("%.2f GB", gb)
}

// Tính tổng dung lượng của 1 thư mục
func getFolderSize(srv *drive.Service, folderID string) int64 {
	var total int64

	// Lấy tất cả file trong thư mục
	query := fmt.Sprintf("'%s' in parents and trashed=false", folderID)
	files, err := srv.Files.List().Q(query).Fields("files(id, mimeType, size)").PageSize(1000).Do()
	if err != nil {
		return 0
	}

	for _, f := range files.Files {
		if f.MimeType == "application/vnd.google-apps.folder" {
			// Nếu là thư mục con -> tính đệ quy
			total += getFolderSize(srv, f.Id)
		} else {
			// Nếu là file -> cộng dung lượng
			total += f.Size
		}
	}

	return total
}

// In danh sách thư mục
func printFolders(srv *drive.Service, parentID string, indent int) {
	// Tìm thư mục
	query := "mimeType='application/vnd.google-apps.folder' and trashed=false"
	if parentID == "" {
		query += " and 'root' in parents"
	} else {
		query += fmt.Sprintf(" and '%s' in parents", parentID)
	}

	folders, err := srv.Files.List().Q(query).Fields("files(id, name, createdTime)").OrderBy("name").Do()
	if err != nil {
		return
	}

	// In từng thư mục
	for _, folder := range folders.Files {
		// Tạo khoảng trắng đầu dòng
		spaces := strings.Repeat("  ", indent)

		// Tính dung lượng
		size := getFolderSize(srv, folder.Id)
		sizeStr := formatSize(size)

		// Lấy ngày tạo (chỉ lấy YYYY-MM-DD)
		date := "N/A"
		if len(folder.CreatedTime) >= 10 {
			date = folder.CreatedTime[:10]
		}

		// In thông tin
		fmt.Printf("%s📁 %-40s | %10s | %s\n", spaces, folder.Name, sizeStr, date)

		// In thư mục con (đệ quy)
		printFolders(srv, folder.Id, indent+1)
	}
}

func main() {
	// Bước 1: Đọc credentials
	config := readCredentials()

	// Bước 2: Lấy token (xác thực)
	token := getToken(config)
	client := config.Client(context.Background(), token)

	// Bước 3: Kết nối Drive API
	srv, err := drive.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		log.Fatal("❌ Không thể kết nối Drive API")
	}

	// Bước 4: In danh sách thư mục
	fmt.Println("=== DANH SÁCH THƯ MỤC GOOGLE DRIVE ===")
	fmt.Println("Format: Tên | Dung lượng | Ngày tạo")
	fmt.Println(strings.Repeat("-", 80))
	printFolders(srv, "", 0)
}