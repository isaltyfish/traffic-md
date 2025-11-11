/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bufio"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var (
	fakeSpeed  int    // 每秒写入的行数
	fakeOutput string // 输出文件路径
	fakeLoop   bool   // 是否循环读取
)

// fakeCmd represents the fake command
var fakeCmd = &cobra.Command{
	Use:   "fake",
	Short: "Simulate nginx log generation",
	Long: `Read log samples from cmd/logsample.txt and write them to /tmp/nginx.log
at a controlled speed to simulate normal nginx log generation.

Example:
  traffic-md fake --speed 100 --output /tmp/nginx.log`,
	Run: func(cmd *cobra.Command, args []string) {
		// 获取日志样本文件路径
		exePath, err := os.Executable()
		if err != nil {
			log.Fatalf("Failed to get executable path: %v", err)
		}
		exeDir := filepath.Dir(exePath)
		sampleFile := filepath.Join(exeDir, "cmd", "logsample.txt")

		// 如果是在开发环境，尝试使用当前工作目录
		if _, err := os.Stat(sampleFile); os.IsNotExist(err) {
			// 尝试从当前目录查找
			sampleFile = filepath.Join(".", "cmd", "logsample.txt")
			if _, err := os.Stat(sampleFile); os.IsNotExist(err) {
				log.Fatalf("Log sample file not found: %s", sampleFile)
			}
		}

		// 打开日志样本文件
		sample, err := os.Open(sampleFile)
		if err != nil {
			log.Fatalf("Failed to open log sample file: %v", err)
		}
		defer sample.Close()

		// 打开或创建输出文件（追加模式）
		output, err := os.OpenFile(fakeOutput, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("Failed to open output file: %v", err)
		}
		defer func() {
			// 程序退出时同步一次，确保数据写入磁盘
			if err := output.Sync(); err != nil {
				log.Printf("Warning: Failed to sync file on exit: %v", err)
			}
			output.Close()
		}()

		// 设置信号处理
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

		// 计算每行之间的延迟（毫秒）
		var delay time.Duration
		if fakeSpeed > 0 {
			delay = time.Second / time.Duration(fakeSpeed)
		} else {
			// 默认速度：每秒 50 行
			delay = 20 * time.Millisecond
		}

		log.Printf("Starting log simulation:")
		log.Printf("  Sample file: %s", sampleFile)
		log.Printf("  Output file: %s", fakeOutput)
		log.Printf("  Speed: %d lines/second (%.2f ms/line)", fakeSpeed, delay.Seconds()*1000)
		log.Printf("Press Ctrl+C to stop...")

		scanner := bufio.NewScanner(sample)
		lineCount := 0

		// 主循环
		for {
			select {
			case <-sigCh:
				log.Printf("\nReceived interrupt signal, stopping...")
				// 确保数据写入磁盘
				if err := output.Sync(); err != nil {
					log.Printf("Warning: Failed to sync file: %v", err)
				}
				log.Printf("Total lines written: %d", lineCount)
				return
			default:
				// 读取一行
				if !scanner.Scan() {
					if err := scanner.Err(); err != nil {
						log.Printf("Error reading sample file: %v", err)
						return
					}
					// 文件读取完毕，同步后退出
					if err := output.Sync(); err != nil {
						log.Printf("Warning: Failed to sync file: %v", err)
					}
					log.Printf("Reached end of file. Total lines written: %d", lineCount)
					return
				}

				line := scanner.Text()
				if line == "" {
					continue
				}

				// 写入输出文件（数据会先进入操作系统的 page cache）
				// 操作系统会定期将数据刷新到磁盘，不需要频繁调用 Sync()
				if _, err := output.WriteString(line + "\n"); err != nil {
					log.Printf("Error writing to output file: %v", err)
					return
				}

				lineCount++

				// 控制写入速度
				time.Sleep(delay)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(fakeCmd)

	fakeCmd.Flags().IntVarP(&fakeSpeed, "speed", "s", 50, "Lines per second to write (0 = as fast as possible)")
	fakeCmd.Flags().StringVarP(&fakeOutput, "output", "o", "/tmp/nginx.log", "Output file path")
	fakeCmd.Flags().BoolVarP(&fakeLoop, "loop", "l", false, "Loop through the sample file continuously")
}
