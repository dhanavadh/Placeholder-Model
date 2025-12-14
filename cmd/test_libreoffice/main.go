package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"DF-PLCH/internal/processor"
)

func main() {
	totalStart := time.Now()

	// Check if LibreOffice is available
	if !processor.IsLibreOfficeAvailable() {
		fmt.Println("✗ LibreOffice not available")
		os.Exit(1)
	}
	fmt.Printf("✓ LibreOffice found at: %s\n", processor.FindLibreOffice())

	// Use the existing template file
	inputFile := "cmd/server/storage/templates/f8a4d8d7-b641-458a-8f5a-bdd8b237ca7e/1765697491_บัตรประชาชน แบบ1 หน้าเดี่ยว.docx"
	outputFile := "/tmp/test_output.docx"

	// Check if input exists
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		fmt.Printf("✗ Input file not found: %s\n", inputFile)
		os.Exit(1)
	}
	fmt.Printf("✓ Input file exists: %s\n", filepath.Base(inputFile))

	// Create LibreOffice processor
	loProc := processor.NewLibreOfficeProcessor(inputFile, outputFile)
	defer loProc.Cleanup()

	// Extract placeholders first
	fmt.Println("\n--- Extracting placeholders ---")
	extractStart := time.Now()
	placeholders, err := loProc.ExtractPlaceholders()
	extractDuration := time.Since(extractStart)
	if err != nil {
		fmt.Printf("✗ Failed to extract placeholders: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Found %d placeholders (took %v)\n", len(placeholders), extractDuration)
	for i, p := range placeholders {
		fmt.Printf("  %d. %s\n", i+1, p)
	}

	// Create test data for replacements
	testData := make(map[string]string)
	for _, p := range placeholders {
		testData[p] = "[REPLACED]"
	}

	// Process with LibreOffice
	fmt.Println("\n--- Processing with LibreOffice ---")
	processStart := time.Now()
	err = loProc.ProcessWithPlaceholders(testData)
	processDuration := time.Since(processStart)
	if err != nil {
		fmt.Printf("✗ Failed to process: %v (took %v)\n", err, processDuration)
		os.Exit(1)
	}
	fmt.Printf("✓ Processing completed (took %v)\n", processDuration)

	// Verify output exists
	if info, err := os.Stat(outputFile); err == nil {
		fmt.Printf("✓ Output file created: %s (%d bytes)\n", outputFile, info.Size())
	} else {
		fmt.Printf("✗ Output file not created: %v\n", err)
		os.Exit(1)
	}

	totalDuration := time.Since(totalStart)
	fmt.Println("\n========================================")
	fmt.Printf("TIMING SUMMARY:\n")
	fmt.Printf("  Extraction:  %v\n", extractDuration)
	fmt.Printf("  Processing:  %v\n", processDuration)
	fmt.Printf("  Total:       %v\n", totalDuration)
	fmt.Println("========================================")
	fmt.Println("\n✓ LibreOffice processor test completed successfully!")
}
