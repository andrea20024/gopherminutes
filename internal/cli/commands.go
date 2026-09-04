// Package cli provides Cobra command implementations.
package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/andrea20024/goferminutes2/internal/config"
	"github.com/andrea20024/goferminutes2/internal/mongo"
	"github.com/andrea20024/goferminutes2/internal/service"
	"github.com/spf13/cobra"
)

// RegisterCommands registers all CLI commands with the root command.
func RegisterCommands(rootCmd *cobra.Command, cfg *config.Config) {
	rootCmd.AddCommand(startCmd(cfg))
	rootCmd.AddCommand(loadCmd(cfg))
	rootCmd.AddCommand(listCmd(cfg))
	rootCmd.AddCommand(statusCmd(cfg))
	rootCmd.AddCommand(getCmd(cfg))
	rootCmd.AddCommand(getAudioCmd(cfg))
	rootCmd.AddCommand(findCmd(cfg))
	rootCmd.AddCommand(chatCmd(cfg))
	rootCmd.AddCommand(retryCmd(cfg))
	rootCmd.AddCommand(deleteCmd(cfg))
}

// getHandlers lazily initializes and returns command handlers.
var getHandlers func(cfg *config.Config) (*service.Handlers, error)

// SetHandlersFactory sets the factory function for creating handlers.
func SetHandlersFactory(f func(cfg *config.Config) (*service.Handlers, error)) {
	getHandlers = f
}

func startCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Register a new user",
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := getHandlers(cfg)
			if err != nil {
				return err
			}

			userIDStr, _ := cmd.Flags().GetString("user-id")
			if userIDStr == "" {
				userIDStr = "1"
			}
			userID, err := parseInt(userIDStr, "user-id")
			if err != nil {
				return err
			}

			user, err := h.UserRepo.GetUserByID(cmd.Context(), userID)
			if err == nil {
				fmt.Printf("User already registered: ID=%d\n", user.ID)
				return nil
			}

			_, err = h.UserRepo.CreateUser(cmd.Context(), userID, "cli_user")
			if err != nil {
				return fmt.Errorf("register user: %w", err)
			}

			fmt.Printf("User registered successfully: ID=%d\n", userID)
			return nil
		},
	}
}

func loadCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "load <path>",
		Short: "Load an audio or text file for processing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := getHandlers(cfg)
			if err != nil {
				return err
			}

			filePath := args[0]

			// Resolve relative path: if only filename, look next to exe
			if !filepath.IsAbs(filePath) {
				exePath, err := os.Executable()
				if err == nil {
					exeDir := filepath.Dir(exePath)
					fullPath := filepath.Join(exeDir, filePath)
					if _, err := os.Stat(fullPath); err == nil {
						filePath = fullPath
					}
				}
			}

			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				return fmt.Errorf("%w: %s", service.ErrFileNotFound, filePath)
			}

			// Validate file format
			if err := service.ValidateFileFormat(filePath); err != nil {
				return err
			}

			fileName := filepath.Base(filePath)
			fileData, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("read file: %w", err)
			}

			fileMIME := service.GetMIMEType(filepath.Ext(fileName))

			userIDStr, _ := cmd.Flags().GetString("user-id")
			if userIDStr == "" {
				userIDStr = "1"
			}
			userID, err := parseInt(userIDStr, "user-id")
			if err != nil {
				return err
			}

			// Find user by ID (CLI mode: user_id matches the CLI flag)
			user, err := h.UserRepo.GetUserByID(cmd.Context(), userID)
			if err != nil {
				// Auto-register if user doesn't exist
				user, err = h.UserRepo.CreateUser(cmd.Context(), userID, "cli_user")
				if err != nil {
					return fmt.Errorf("create user: %w", err)
				}
			}

			meeting, task, err := h.Service.StartProcessing(cmd.Context(), user.ID, filePath, fileData, fileMIME)
			if err != nil {
				return fmt.Errorf("load file: %w", err)
			}

			fmt.Printf("Meeting created: ID=%d\n", meeting.ID)
			fmt.Printf("Task created: ID=%d, Status=%s\n", task.ID, task.Status)
			fmt.Printf("File: %s (%d bytes, %s)\n", fileName, len(fileData), fileMIME)
			if h.GridFS != nil && meeting.GridFSID != nil {
				fmt.Printf("Audio stored in GridFS: %s\n", *meeting.GridFSID)
			}
			fmt.Println("Processing started in background. Use 'status <meeting_id>' to check progress.")
			return nil
		},
	}
}

func listCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all meetings for the current user",
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := getHandlers(cfg)
			if err != nil {
				return err
			}

			userIDStr, _ := cmd.Flags().GetString("user-id")
			if userIDStr == "" {
				userIDStr = "1"
			}
			userID, err := parseInt(userIDStr, "user-id")
			if err != nil {
				return err
			}

			meetings, err := h.Service.ListMeetings(cmd.Context(), userID)
			if err != nil {
				return fmt.Errorf("list meetings: %w", err)
			}

			if len(meetings) == 0 {
				fmt.Println("No meetings found.")
				return nil
			}

			fmt.Printf("%-5s %-25s %-15s %-40s %-25s\n", "ID", "File Name", "Status", "Summary", "Created")
			fmt.Println(strings.Repeat("-", 110))
			for _, m := range meetings {
				summary := ""
				if m.Summary != nil {
					summary = truncate(*m.Summary, 40)
				}
				fmt.Printf("%-5d %-25s %-15s %-40s %-25s\n",
					m.ID, m.FileName, m.Status, summary, m.CreatedAt.Format("2006-01-02 15:04"))
			}
			return nil
		},
	}
}

func statusCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "status <id>",
		Short: "Get processing status of a meeting",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := getHandlers(cfg)
			if err != nil {
				return err
			}

			meetingID, err := parseInt(args[0], "meeting ID")
			if err != nil {
				return err
			}

			userIDStr, _ := cmd.Flags().GetString("user-id")
			if userIDStr == "" {
				userIDStr = "1"
			}
			userID, err := parseInt(userIDStr, "user-id")
			if err != nil {
				return err
			}

			meeting, err := h.Service.GetMeeting(cmd.Context(), meetingID, userID)
			if err != nil {
				return formatError(err)
			}

			fmt.Printf("Meeting ID:     %d\n", meeting.ID)
			fmt.Printf("File Name:      %s\n", meeting.FileName)
			fmt.Printf("Status:         %s\n", meeting.Status)
			fmt.Printf("Created At:     %s\n", meeting.CreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("Updated At:     %s\n", meeting.UpdatedAt.Format("2006-01-02 15:04:05"))
			if meeting.ErrorMessage != nil {
				fmt.Printf("Error Message:  %s\n", *meeting.ErrorMessage)
			}
			return nil
		},
	}
}

func getCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get full transcription of a meeting",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := getHandlers(cfg)
			if err != nil {
				return err
			}

			meetingID, err := parseInt(args[0], "meeting ID")
			if err != nil {
				return err
			}

			userIDStr, _ := cmd.Flags().GetString("user-id")
			if userIDStr == "" {
				userIDStr = "1"
			}
			userID, err := parseInt(userIDStr, "user-id")
			if err != nil {
				return err
			}

			meeting, err := h.Service.GetMeeting(cmd.Context(), meetingID, userID)
			if err != nil {
				return formatError(err)
			}

			fmt.Printf("=== Meeting: %s (ID: %d) ===\n", meeting.FileName, meeting.ID)
			fmt.Printf("Status: %s\n\n", meeting.Status)

			if meeting.Transcription != nil {
				fmt.Println("=== Transcription ===")
				fmt.Println(*meeting.Transcription)
			} else {
				fmt.Println("Transcription not available yet.")
			}

			if meeting.Summary != nil {
				fmt.Printf("\n=== Summary ===\n%s\n", *meeting.Summary)
			}
			return nil
		},
	}
}

func findCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "find <keyword>",
		Short: "Search meetings by keyword",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := getHandlers(cfg)
			if err != nil {
				return err
			}

			keyword := args[0]

			userIDStr, _ := cmd.Flags().GetString("user-id")
			if userIDStr == "" {
				userIDStr = "1"
			}
			userID, err := parseInt(userIDStr, "user-id")
			if err != nil {
				return err
			}

			meetings, err := h.Service.SearchMeetings(cmd.Context(), userID, keyword)
			if err != nil {
				return fmt.Errorf("search: %w", err)
			}

			if len(meetings) == 0 {
				fmt.Printf("No meetings found with keyword: %s\n", keyword)
				return nil
			}

			fmt.Printf("Found %d meeting(s) with keyword: %s\n\n", len(meetings), keyword)
			for _, m := range meetings {
				fmt.Printf("[%d] %s (Status: %s, Created: %s)\n",
					m.ID, m.FileName, m.Status, m.CreatedAt.Format("2006-01-02"))
				if m.Summary != nil {
					fmt.Printf("    Summary: %s\n", *m.Summary)
				}
			}
			return nil
		},
	}
}

func chatCmd(cfg *config.Config) *cobra.Command {
	var meetingID int

	cmd := &cobra.Command{
		Use:   "chat <text>",
		Short: "Ask a question about meeting materials",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := getHandlers(cfg)
			if err != nil {
				return err
			}

			text := args[0]

			userIDStr, _ := cmd.Flags().GetString("user-id")
			if userIDStr == "" {
				userIDStr = "1"
			}
			userID, err := parseInt(userIDStr, "user-id")
			if err != nil {
				return err
			}

			var meetingPtr *int
			if meetingID > 0 {
				meetingPtr = &meetingID
			}

			answer, err := h.Service.Chat(cmd.Context(), userID, text, meetingPtr)
			if err != nil {
				return fmt.Errorf("chat: %w", err)
			}

			fmt.Printf("Answer: %s\n", answer)
			return nil
		},
	}

	cmd.Flags().IntVar(&meetingID, "meeting", 0, "Meeting ID to ask about (optional)")
	return cmd
}

func retryCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "retry <id>",
		Short: "Retry processing a failed meeting",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := getHandlers(cfg)
			if err != nil {
				return err
			}

			meetingID, err := parseInt(args[0], "meeting ID")
			if err != nil {
				return err
			}

			userIDStr, _ := cmd.Flags().GetString("user-id")
			if userIDStr == "" {
				userIDStr = "1"
			}
			userID, err := parseInt(userIDStr, "user-id")
			if err != nil {
				return err
			}

			meeting, err := h.Service.RetryProcessing(cmd.Context(), meetingID, userID)
			if err != nil {
				return formatError(err)
			}

			fmt.Printf("Retrying meeting: ID=%d\n", meeting.ID)
			return nil
		},
	}
}

func deleteCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a meeting and its associated data",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := getHandlers(cfg)
			if err != nil {
				return err
			}

			meetingID, err := parseInt(args[0], "meeting ID")
			if err != nil {
				return err
			}

			userIDStr, _ := cmd.Flags().GetString("user-id")
			if userIDStr == "" {
				userIDStr = "1"
			}
			userID, err := parseInt(userIDStr, "user-id")
			if err != nil {
				return err
			}

			// Verify the meeting exists and belongs to the user
			_, err = h.Service.GetMeeting(cmd.Context(), meetingID, userID)
			if err != nil {
				return formatError(err)
			}

			// Delete meeting (cascading delete handled by database FK constraints)
			err = h.Repository.DeleteMeeting(cmd.Context(), meetingID)
			if err != nil {
				return fmt.Errorf("delete meeting: %w", err)
			}

			fmt.Printf("Meeting %d deleted successfully\n", meetingID)
			return nil
		},
	}
}

func getAudioCmd(cfg *config.Config) *cobra.Command {
	var outputPath string

	cmd := &cobra.Command{
		Use:   "get-audio <id>",
		Short: "Download the audio file for a meeting from GridFS",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := getHandlers(cfg)
			if err != nil {
				return err
			}

			if h.GridFS == nil {
				return service.ErrGridFSNotConfigured
			}

			meetingID, err := parseInt(args[0], "meeting ID")
			if err != nil {
				return err
			}

			userIDStr, _ := cmd.Flags().GetString("user-id")
			if userIDStr == "" {
				userIDStr = "1"
			}
			userID, err := parseInt(userIDStr, "user-id")
			if err != nil {
				return err
			}

			meeting, err := h.Service.GetMeeting(cmd.Context(), meetingID, userID)
			if err != nil {
				return err
			}

			if meeting.GridFSID == nil {
				return fmt.Errorf("no audio file stored for this meeting (GridFS not configured or file not uploaded)")
			}

			gridfsID, err := mongo.ParseGridFSID(*meeting.GridFSID)
			if err != nil {
				return fmt.Errorf("invalid GridFS ID: %w", err)
			}

			if outputPath == "" {
				outputPath = fmt.Sprintf("meeting_%d_audio.mp3", meetingID)
			}

			if err := h.GridFS.DownloadFile(cmd.Context(), gridfsID, outputPath); err != nil {
				return fmt.Errorf("download audio: %w", err)
			}

			fmt.Printf("Audio file downloaded: %s (meeting ID: %d)\n", outputPath, meetingID)
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path (default: meeting_<id>_audio.mp3)")
	return cmd
}

// parseInt parses a string into an integer, returning a descriptive error.
func parseInt(s, field string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("missing required value for %s", field)
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid value for %s: %q is not a valid integer", field, s)
	}
	return n, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// formatError formats an error with a user-friendly message based on the error type.
func formatError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, service.ErrMeetingNotFound):
		return fmt.Errorf("meeting not found. Please check the ID and try again")
	case errors.Is(err, service.ErrAccessDenied):
		return fmt.Errorf("access denied: you can only access your own meetings")
	case errors.Is(err, service.ErrUnsupportedFormat):
		return fmt.Errorf("unsupported file format. Supported formats: mp3, wav, ogg, flac")
	case errors.Is(err, service.ErrFileNotFound):
		return fmt.Errorf("file not found. Please check the path and try again")
	case errors.Is(err, service.ErrRetryOnNonFailedMeeting):
		return fmt.Errorf("retry is only available for failed meetings. Current status: %v", err)
	case errors.Is(err, service.ErrGridFSFileNotFound):
		return fmt.Errorf("audio file not found in storage. Please reload the meeting")
	case errors.Is(err, service.ErrGridFSNotConfigured):
		return fmt.Errorf("GridFS is not configured, cannot perform this operation")
	case errors.Is(err, service.ErrServiceShuttingDown):
		return fmt.Errorf("service is shutting down, please try again later")
	case errors.Is(err, service.ErrContextTimeout):
		return fmt.Errorf("operation timed out, please try again")
	case errors.Is(err, service.ErrSpeechClientUnavailable):
		return fmt.Errorf("speech recognition service unavailable. Please try again later")
	case errors.Is(err, service.ErrLLMClientUnavailable):
		return fmt.Errorf("LLM service unavailable. Please try again later")
	case errors.Is(err, service.ErrDatabaseUnavailable):
		return fmt.Errorf("database connection failed. Please try again later")
	case errors.Is(err, service.ErrInvalidMeetingID):
		return fmt.Errorf("invalid meeting ID. Please provide a valid numeric ID")
	case errors.Is(err, service.ErrUnknownCommand):
		return fmt.Errorf("unknown command. Run 'goferminutes2 --help' for available commands")
	case errors.Is(err, service.ErrMissingArguments):
		return fmt.Errorf("missing required arguments. Run command with --help for usage")
	default:
		return fmt.Errorf("an unexpected error occurred: %v", err)
	}
}
