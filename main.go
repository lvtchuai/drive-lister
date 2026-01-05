package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// Lấy token từ web
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	config.RedirectURL = "urn:ietf:wg:oauth:2.0:oob"
	
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Println("\n=== HƯỚNG DẪN XÁC THỰC ===")
	fmt.Println("1. Mở URL sau trong trình duyệt:")
	fmt.Printf("\n%v\n\n", authURL)
	fmt.Println("2. Đăng nhập và cho phép quyền truy cập")
	fmt.Println("3. Copy mã xác thực hiển thị trên trang")
	fmt.Print("4. Dán mã vào đây: ")

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Không thể đọc mã xác thực: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Không thể lấy token: %v", err)
	}
	return tok
}

// Lưu token vào file
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Đang lưu token vào: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Không thể lưu token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// Đọc token từ file
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Lấy client OAuth2
func getClient(config *oauth2.Config) *http.Client {
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Tính dung lượng thư mục
func calculateFolderSize(srv *drive.Service, folderID string) (int64, error) {
	var totalSize int64
	pageToken := ""
	
	for {
		query := fmt.Sprintf("'%s' in parents and trashed=false", folderID)
		
		call := srv.Files.List().
			Q(query).
			Fields("nextPageToken, files(id, mimeType, size)").
			PageSize(1000)
		
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		
		r, err := call.Do()
		if err != nil {
			return 0, err
		}
		
		for _, file := range r.Files {
			if file.MimeType == "application/vnd.google-apps.folder" {
				// Đệ quy tính dung lượng thư mục con
				subSize, err := calculateFolderSize(srv, file.Id)
				if err == nil {
					totalSize += subSize
				}
			} else {
				totalSize += file.Size
			}
		}
		
		pageToken = r.NextPageToken
		if pageToken == "" {
			break
		}
	}
	
	return totalSize, nil
}

// Chuyển đổi bytes sang đơn vị dễ đọc
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// Liệt kê thư mục
func listFolders(srv *drive.Service, parentID string, level int) error {
	query := "mimeType='application/vnd.google-apps.folder'"
	if parentID != "" {
		query += fmt.Sprintf(" and '%s' in parents", parentID)
	} else {
		query += " and 'root' in parents"
	}
	query += " and trashed=false"

	r, err := srv.Files.List().
		Q(query).
		Fields("files(id, name, createdTime, modifiedTime)").
		OrderBy("name").
		Do()
	
	if err != nil {
		return fmt.Errorf("không thể lấy danh sách thư mục: %v", err)
	}

	indent := ""
	for i := 0; i < level; i++ {
		indent += "  "
	}

	for _, folder := range r.Files {
		// Tính dung lượng thư mục
		size, err := calculateFolderSize(srv, folder.Id)
		sizeStr := "0 B"
		if err == nil {
			sizeStr = formatSize(size)
		}
		
		// Format ngày tạo
		createdTime := "N/A"
		if folder.CreatedTime != "" {
			createdTime = folder.CreatedTime[:10] // Lấy YYYY-MM-DD
		}
		
		fmt.Printf("%s📁 %-40s | %10s | %s\n", 
			indent, 
			folder.Name, 
			sizeStr, 
			createdTime)
		
		// Đệ quy liệt kê thư mục con
		listFolders(srv, folder.Id, level+1)
	}

	return nil
}

func main() {
	ctx := context.Background()

	// Đọc credentials từ file
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Không thể đọc file credentials.json: %v\n", err)
		log.Fatal("Vui lòng tải credentials.json từ Google Cloud Console")
	}

	config, err := google.ConfigFromJSON(b, drive.DriveReadonlyScope)
	if err != nil {
		log.Fatalf("Không thể parse credentials: %v", err)
	}

	client := getClient(config)

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Không thể tạo Drive service: %v", err)
	}

	fmt.Println("=== DANH SÁCH THƯ MỤC GOOGLE DRIVE ===")
	fmt.Println("Format: Tên thư mục | Dung lượng | Ngày tạo\n")
	fmt.Println(strings.Repeat("-", 80))
	
	if err := listFolders(srv, "", 0); err != nil {
		log.Fatalf("Lỗi: %v", err)
	}
}