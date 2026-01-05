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
		log.Fatal("Không tìm thấy file credentials.json")
	}

	config, err := google.ConfigFromJSON(data, drive.DriveScope)
	if err != nil {
		log.Fatal("File credentials.json không hợp lệ")
	}

	return config
}

// Lấy token
func getToken(config *oauth2.Config) *oauth2.Token {
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
		log.Fatal("Mã xác thực không đúng")
	}

	// Lưu token
	data, _ = json.Marshal(token)
	os.WriteFile(tokenFile, data, 0600)
	fmt.Println("Xác thực thành công!\n")

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

	query := fmt.Sprintf("'%s' in parents and trashed=false", folderID)
	files, err := srv.Files.List().Q(query).Fields("files(id, mimeType, size)").PageSize(1000).Do()
	if err != nil {
		return 0
	}

	for _, f := range files.Files {
		if f.MimeType == "application/vnd.google-apps.folder" {
			total += getFolderSize(srv, f.Id)
		} else {
			total += f.Size
		}
	}

	return total
}

// In danh sách thư mục
func printFolders(srv *drive.Service, parentID string, indent int) {
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

	for _, folder := range folders.Files {
		spaces := strings.Repeat("  ", indent)

		size := getFolderSize(srv, folder.Id)
		sizeStr := formatSize(size)

		date := "N/A"
		if len(folder.CreatedTime) >= 10 {
			date = folder.CreatedTime[:10]
		}

		fmt.Printf("%s %-40s | %10s | %s\n", spaces, folder.Name, sizeStr, date)

		printFolders(srv, folder.Id, indent+1)
	}
}

// Tạo thư mục mới
func createFolder(srv *drive.Service, folderName string, parentID string) {
	file := &drive.File{
		Name:     folderName,
		MimeType: "application/vnd.google-apps.folder",
	}

	if parentID != "" {
		file.Parents = []string{parentID}
	}

	createdFile, err := srv.Files.Create(file).Do()
	if err != nil {
		fmt.Printf("Không thể tạo thư mục: %v\n", err)
		return
	}

	fmt.Printf("Thư mục '%s' đã được tạo thành công! (ID: %s)\n", folderName, createdFile.Id)
}

// Tìm thư mục theo tên
func findFolderByName(srv *drive.Service, folderName string, parentID string) (string, error) {
	query := fmt.Sprintf("name='%s' and mimeType='application/vnd.google-apps.folder' and trashed=false", folderName)
	if parentID != "" {
		query += fmt.Sprintf(" and '%s' in parents", parentID)
	}

	files, err := srv.Files.List().Q(query).Fields("files(id, name)").Do()
	if err != nil {
		return "", err
	}

	if len(files.Files) == 0 {
		return "", fmt.Errorf("không tìm thấy thư mục '%s'", folderName)
	}

	if len(files.Files) > 1 {
		fmt.Println("\nTìm thấy nhiều thư mục cùng tên:")
		for i, f := range files.Files {
			fmt.Printf("%d. %s (ID: %s)\n", i+1, f.Name, f.Id)
		}
		fmt.Print("Chọn số: ")
		var choice int
		fmt.Scan(&choice)
		if choice < 1 || choice > len(files.Files) {
			return "", fmt.Errorf("lựa chọn không hợp lệ")
		}
		return files.Files[choice-1].Id, nil
	}

	return files.Files[0].Id, nil
}

// Xoá thư mục
func deleteFolder(srv *drive.Service, folderID string) {
	// Lấy tên thư mục trước khi xoá
	file, err := srv.Files.Get(folderID).Fields("name").Do()
	if err != nil {
		fmt.Printf("Không tìm thấy thư mục với ID: %s\n", folderID)
		return
	}

	// Xoá thư mục
	err = srv.Files.Delete(folderID).Do()
	if err != nil {
		fmt.Printf("Không thể xoá thư mục '%s': %v\n", file.Name, err)
		return
	}

	fmt.Printf("Thư mục '%s' đã được xoá thành công!\n", file.Name)
}

// Đổi tên thư mục
func renameFolder(srv *drive.Service, folderID string, newName string) {
	file, err := srv.Files.Get(folderID).Fields("name").Do()
	if err != nil {
		fmt.Printf("Không tìm thấy thư mục với ID: %s\n", folderID)
		return
	}

	oldName := file.Name

	file.Name = newName
	_, err = srv.Files.Update(folderID, file).Do()
	if err != nil {
		fmt.Printf("Không thể đổi tên thư mục: %v\n", err)
		return
	}

	fmt.Printf("Đổi tên thư mục từ '%s' thành '%s' thành công!\n", oldName, newName)
}

func main() {
	config := readCredentials()

	token := getToken(config)
	client := config.Client(context.Background(), token)

	srv, err := drive.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		log.Fatal("Không thể kết nối Drive API")
	}

	fmt.Println("\n=== CHỌN DRIVE ===")
	fmt.Println("1. Drive của tôi (My Drive)")
	fmt.Println("2. Drive được chia sẻ (Shared Drive)")
	fmt.Println("3. Thư mục cụ thể (nhập ID)")
	fmt.Print("\nChọn (1/2/3): ")

	var choice string
	fmt.Scan(&choice)

	var folderID string
	var driveName string

	switch choice {
	case "1":
		folderID = ""
		driveName = "My Drive"
	case "2":
		drives, err := srv.Drives.List().Do()
		if err != nil {
			log.Fatal("Không thể lấy danh sách Shared Drives")
		}

		if len(drives.Drives) == 0 {
			fmt.Println("Không có Shared Drive nào")
			return
		}

		fmt.Println("\n=== SHARED DRIVES ===")
		for i, d := range drives.Drives {
			fmt.Printf("%d. %s (ID: %s)\n", i+1, d.Name, d.Id)
		}
		fmt.Print("\nChọn số: ")

		var num int
		fmt.Scan(&num)

		if num < 1 || num > len(drives.Drives) {
			log.Fatal("Lựa chọn không hợp lệ")
		}

		folderID = drives.Drives[num-1].Id
		driveName = drives.Drives[num-1].Name

	case "3":
		fmt.Print("Nhập Folder ID: ")
		fmt.Scan(&folderID)
		driveName = "Thư mục tùy chỉnh"
	default:
		log.Fatal("Lựa chọn không hợp lệ")
	}

	for {
		fmt.Printf("\n=== MENU CHÍNH ===\n")
		fmt.Println("1. Xem danh sách thư mục")
		fmt.Println("2. Tạo thư mục mới")
		fmt.Println("3. Đổi tên thư mục")
		fmt.Println("4. Xoá thư mục")
		fmt.Println("5. Thoát")
		fmt.Print("\nChọn (1-5): ")

		var menuChoice string
		fmt.Scan(&menuChoice)

		switch menuChoice {
		case "1":
			fmt.Printf("\n=== DANH SÁCH THƯ MỤC: %s ===\n", driveName)
			fmt.Println("Format: Tên | Dung lượng | Ngày tạo")
			fmt.Println(strings.Repeat("-", 80))
			printFolders(srv, folderID, 0)

		case "2":
			fmt.Print("\nNhập tên thư mục mới: ")
			var folderName string
			fmt.Scan(&folderName)
			createFolder(srv, folderName, folderID)

		case "3":
			fmt.Print("\nNhập tên thư mục cần đổi tên: ")
			var targetFolderName string
			fmt.Scan(&targetFolderName)
			targetFolderID, err := findFolderByName(srv, targetFolderName, folderID)
			if err != nil {
				fmt.Printf("%v\n", err)
			} else {
				fmt.Print("Nhập tên mới: ")
				var newName string
				fmt.Scan(&newName)
				renameFolder(srv, targetFolderID, newName)
			}

		case "4":
			fmt.Print("\nNhập tên thư mục cần xoá: ")
			var targetFolderName string
			fmt.Scan(&targetFolderName)
			targetFolderID, err := findFolderByName(srv, targetFolderName, folderID)
			if err != nil {
				fmt.Printf("%v\n", err)
			} else {
				fmt.Print("Bạn chắc chắn muốn xoá? (y/n): ")
				var confirm string
				fmt.Scan(&confirm)
				if confirm == "y" || confirm == "Y" {
					deleteFolder(srv, targetFolderID)
				} else {
					fmt.Println("Đã hủy thao tác")
				}
			}

		case "5":
			fmt.Println("\nTạm biệt!")
			return

		default:
			fmt.Println("Lựa chọn không hợp lệ")
		}
	}
}
