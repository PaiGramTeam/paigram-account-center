package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"paigram/internal/logging"
	"paigram/internal/openapidoc"
)

var openAPIOutputPath string

var openAPICmd = &cobra.Command{
	Use:   "openapi",
	Short: "Write the OpenAPI document",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		document, err := openapidoc.Generate()
		if err != nil {
			logging.Error("OpenAPI document generation failed", zap.Error(err))
			return err
		}
		if err := writeOpenAPIDocument(openAPIOutputPath, document); err != nil {
			logging.Error("OpenAPI document write failed", zap.String("path", openAPIOutputPath), zap.Error(err))
			return err
		}
		logging.Info("OpenAPI document written", zap.String("path", openAPIOutputPath))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(openAPICmd)
	openAPICmd.Flags().StringVar(&openAPIOutputPath, "out", "", "output JSON path")
	_ = openAPICmd.MarkFlagRequired("out")
}

func writeOpenAPIDocument(path string, contents []byte) (returnErr error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create OpenAPI output directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".openapi-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary OpenAPI document: %w", err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			returnErr = errors.Join(returnErr, temporary.Close())
		}
		if returnErr != nil {
			_ = os.Remove(temporaryPath)
		}
	}()

	if _, err := temporary.Write(contents); err != nil {
		return fmt.Errorf("write temporary OpenAPI document: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		return fmt.Errorf("set OpenAPI document permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync OpenAPI document: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close OpenAPI document: %w", err)
	}
	closed = true
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace OpenAPI document: %w", err)
	}
	return nil
}
