package bot

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// LogEntry 日志条目
type LogEntry struct {
	ConversationID string
	UserID         string
	Content        string
	Timestamp      time.Time
}

// ChatLogger 异步聊天记录日志管理器
type ChatLogger struct {
	logDir     string
	logQueue   chan LogEntry       // 异步日志队列
	fileMap    map[string]*logFile // conversationID -> logFile
	fileMutex  sync.RWMutex
	workerWG   sync.WaitGroup // 工作协程等待组
	shutdownCh chan struct{}  // 关闭信号

	// 统计信息（性能开销极小，对监控有价值）
	totalLogged  uint64 // 成功记录的日志数
	totalDropped uint64 // 因队列满而丢弃的日志数

	// 配置参数
	queueSize     int           // 队列大小
	batchSize     int           // 批量写入大小
	flushInterval time.Duration // 刷新间隔
}

// logFile 包装日志文件和缓冲写入器
type logFile struct {
	file       *os.File
	writer     *bufio.Writer
	lastAccess time.Time
}

// NewChatLogger 创建异步聊天日志记录器
func NewChatLogger(logDir string) (*ChatLogger, error) {
	// 确保日志目录存在
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}

	logger := &ChatLogger{
		logDir:        logDir,
		logQueue:      make(chan LogEntry, 10000), // 10k 缓冲队列
		fileMap:       make(map[string]*logFile),
		shutdownCh:    make(chan struct{}),
		queueSize:     10000,
		batchSize:     100,
		flushInterval: 5 * time.Second,
	}

	// 启动异步日志处理器
	logger.workerWG.Add(1)
	go logger.processLogs()

	// 启动定期维护任务
	logger.workerWG.Add(1)
	go logger.maintenance()

	return logger, nil
}

// LogMessage 异步记录用户消息（非阻塞）
func (cl *ChatLogger) LogMessage(conversationID, userID, content string) error {
	entry := LogEntry{
		ConversationID: conversationID,
		UserID:         userID,
		Content:        content,
		Timestamp:      time.Now(),
	}

	// 非阻塞写入队列
	select {
	case cl.logQueue <- entry:
		atomic.AddUint64(&cl.totalLogged, 1)
		return nil
	default:
		// 队列满时直接丢弃，避免阻塞
		atomic.AddUint64(&cl.totalDropped, 1)
		return nil // 不返回错误，避免影响主流程
	}
}

// processLogs 后台处理日志写入
func (cl *ChatLogger) processLogs() {
	defer cl.workerWG.Done()

	// 批量缓存
	batch := make(map[string][]LogEntry)
	ticker := time.NewTicker(cl.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case entry, ok := <-cl.logQueue:
			if !ok {
				// 队列关闭，写入剩余批量数据
				cl.writeBatches(batch)
				return
			}

			// 添加到批量缓存
			batch[entry.ConversationID] = append(batch[entry.ConversationID], entry)

			// 检查是否需要刷新
			if cl.shouldFlush(batch) {
				cl.writeBatches(batch)
				batch = make(map[string][]LogEntry)
			}

		case <-ticker.C:
			// 定时刷新
			if len(batch) > 0 {
				cl.writeBatches(batch)
				batch = make(map[string][]LogEntry)
			}

		case <-cl.shutdownCh:
			// 收到关闭信号，处理剩余数据
			// 先处理队列中剩余的数据
			close(cl.logQueue)
			for entry := range cl.logQueue {
				batch[entry.ConversationID] = append(batch[entry.ConversationID], entry)
			}
			cl.writeBatches(batch)
			return
		}
	}
}

// shouldFlush 判断是否应该刷新批量缓存
func (cl *ChatLogger) shouldFlush(batch map[string][]LogEntry) bool {
	totalEntries := 0
	for _, entries := range batch {
		totalEntries += len(entries)
		if totalEntries >= cl.batchSize {
			return true
		}
	}
	return false
}

// writeBatches 批量写入日志
func (cl *ChatLogger) writeBatches(batches map[string][]LogEntry) {
	for conversationID, entries := range batches {
		cl.writeEntries(conversationID, entries)
	}
}

// writeEntries 写入一批日志条目到指定会话文件
func (cl *ChatLogger) writeEntries(conversationID string, entries []LogEntry) {
	lf, err := cl.getOrCreateLogFile(conversationID)
	if err != nil {
		fmt.Printf("获取日志文件失败 [%s]: %v\n", conversationID, err)
		return
	}

	// 批量写入
	for _, entry := range entries {
		logLine := fmt.Sprintf("[%s]%s:%s\n",
			entry.Timestamp.Format("2006-01-02 15:04:05"),
			entry.UserID,
			entry.Content)

		if _, err := lf.writer.WriteString(logLine); err != nil {
			fmt.Printf("写入日志失败 [%s]: %v\n", conversationID, err)
			break
		}
	}

	// 更新最后访问时间
	lf.lastAccess = time.Now()
}

// getOrCreateLogFile 获取或创建日志文件
func (cl *ChatLogger) getOrCreateLogFile(conversationID string) (*logFile, error) {
	cl.fileMutex.RLock()
	if lf, exists := cl.fileMap[conversationID]; exists {
		cl.fileMutex.RUnlock()
		return lf, nil
	}
	cl.fileMutex.RUnlock()

	// 需要创建新文件
	cl.fileMutex.Lock()
	defer cl.fileMutex.Unlock()

	// 双重检查
	if lf, exists := cl.fileMap[conversationID]; exists {
		return lf, nil
	}

	// 构建文件路径
	filename := fmt.Sprintf("%s.log", conversationID)
	filepath := filepath.Join(cl.logDir, filename)

	// 以追加模式打开文件
	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件失败: %w", err)
	}

	// 创建大缓冲写入器（64KB）
	writer := bufio.NewWriterSize(file, 65536)

	lf := &logFile{
		file:       file,
		writer:     writer,
		lastAccess: time.Now(),
	}

	cl.fileMap[conversationID] = lf

	// 写入会话开始标记
	startLine := fmt.Sprintf("\n=== 会话开始: %s ===\n", time.Now().Format("2006-01-02 15:04:05"))
	writer.WriteString(startLine)

	return lf, nil
}

// maintenance 定期维护任务
func (cl *ChatLogger) maintenance() {
	defer cl.workerWG.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cl.flushAllFiles()
			cl.printStats()

		case <-cl.shutdownCh:
			return
		}
	}
}

// flushAllFiles 刷新所有文件的缓冲区
func (cl *ChatLogger) flushAllFiles() {
	cl.fileMutex.RLock()
	defer cl.fileMutex.RUnlock()

	for conversationID, lf := range cl.fileMap {
		if err := lf.writer.Flush(); err != nil {
			fmt.Printf("刷新日志文件 %s 失败: %v\n", conversationID, err)
		}
	}
}

// printStats 打印统计信息（仅在需要时）
func (cl *ChatLogger) printStats() {
	logged := atomic.LoadUint64(&cl.totalLogged)
	dropped := atomic.LoadUint64(&cl.totalDropped)
	queueLen := len(cl.logQueue)

	// 只在有问题时打印，避免日志噪音
	if dropped > 0 || queueLen > cl.queueSize/2 {
		fmt.Printf("📊 日志统计 - 已记录: %d, 丢弃: %d, 队列长度: %d/%d\n",
			logged, dropped, queueLen, cl.queueSize)
	}
}

// Close 优雅关闭日志记录器
func (cl *ChatLogger) Close() error {
	fmt.Println("正在关闭日志记录器...")

	// 发送关闭信号
	close(cl.shutdownCh)

	// 等待工作协程完成
	cl.workerWG.Wait()

	// 最后刷新并关闭所有文件
	cl.fileMutex.Lock()
	defer cl.fileMutex.Unlock()

	for conversationID, lf := range cl.fileMap {
		// 写入会话结束标记
		endLine := fmt.Sprintf("=== 会话结束: %s ===\n\n", time.Now().Format("2006-01-02 15:04:05"))
		lf.writer.WriteString(endLine)

		// 刷新缓冲区
		if err := lf.writer.Flush(); err != nil {
			fmt.Printf("刷新日志文件 %s 失败: %v\n", conversationID, err)
		}

		// 关闭文件
		if err := lf.file.Close(); err != nil {
			fmt.Printf("关闭日志文件 %s 失败: %v\n", conversationID, err)
		}
	}

	// 打印最终统计
	logged := atomic.LoadUint64(&cl.totalLogged)
	dropped := atomic.LoadUint64(&cl.totalDropped)
	if dropped > 0 {
		fmt.Printf("⚠️  日志记录器已关闭 - 总计记录: %d, 丢弃: %d\n", logged, dropped)
	} else {
		fmt.Printf("✅ 日志记录器已关闭 - 总计记录: %d\n", logged)
	}

	return nil
}

// GetStats 获取统计信息（供外部监控使用）
func (cl *ChatLogger) GetStats() (logged uint64, dropped uint64, queueLen int) {
	return atomic.LoadUint64(&cl.totalLogged),
		atomic.LoadUint64(&cl.totalDropped),
		len(cl.logQueue)
}
