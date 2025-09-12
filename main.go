package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// 颜色代码
const (
	ColorReset  = "\033[0m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
)

func main() {
	fmt.Printf("%s╔══════════════════════════════════════════╗%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s║           AI-Body 示例选择器             ║%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s║        企业微信智能机器人框架            ║%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s╚══════════════════════════════════════════╝%s\n", ColorCyan, ColorReset)
	fmt.Println()

	examples := []Example{
		{
			Name:        "简单对话示例",
			Description: "基础的命令行对话机器人",
			Path:        "examples/simple-chat",
			Color:       ColorGreen,
		},
		{
			Name:        "流式对话示例",
			Description: "实时流式传输对话，支持逐字符显示",
			Path:        "examples/streaming-chat",
			Color:       ColorPurple,
		},
		{
			Name:        "MCP配置驱动智能体",
			Description: "配置文件驱动的智能体，支持MCP工具集成",
			Path:        "examples/mcp-config-agent",
			Color:       ColorCyan,
		},
	}

	for {
		displayMenu(examples)

		scanner := bufio.NewScanner(os.Stdin)
		fmt.Printf("%s请选择示例 (1-%d) 或输入 'quit' 退出: %s", ColorBlue, len(examples), ColorReset)

		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())

		if input == "quit" || input == "exit" || input == "q" {
			fmt.Printf("%s再见！%s\n", ColorGreen, ColorReset)
			break
		}

		choice, err := strconv.Atoi(input)
		if err != nil || choice < 1 || choice > len(examples) {
			fmt.Printf("%s无效选择，请输入 1-%d%s\n\n", ColorYellow, len(examples), ColorReset)
			continue
		}

		selectedExample := examples[choice-1]
		runExample(selectedExample)
	}
}

type Example struct {
	Name        string
	Description string
	Path        string
	Color       string
}

func displayMenu(examples []Example) {
	fmt.Printf("%s可用示例:%s\n", ColorCyan, ColorReset)
	fmt.Println()

	for i, example := range examples {
		fmt.Printf("%s[%d] %s%s%s\n", ColorBlue, i+1, example.Color, example.Name, ColorReset)
		fmt.Printf("    %s%s%s\n", ColorYellow, example.Description, ColorReset)
		fmt.Printf("    路径: %s\n", example.Path)
		fmt.Println()
	}
}

func runExample(example Example) {
	fmt.Printf("%s正在运行: %s%s%s\n", ColorGreen, example.Color, example.Name, ColorReset)
	fmt.Printf("%s路径: %s%s\n", ColorBlue, example.Path, ColorReset)
	fmt.Printf("%s%s%s\n", ColorYellow, strings.Repeat("=", 50), ColorReset)

	cmd := exec.Command("go", "run", "main.go")
	cmd.Dir = example.Path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err := cmd.Run()
	if err != nil {
		fmt.Printf("%s运行示例时出错: %v%s\n", ColorYellow, err, ColorReset)
	}

	fmt.Printf("\n%s%s%s\n", ColorYellow, strings.Repeat("=", 50), ColorReset)
	fmt.Printf("%s示例运行完毕，按 Enter 返回菜单...%s", ColorGreen, ColorReset)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()

	// 清屏
	fmt.Print("\033[2J\033[H")
}
