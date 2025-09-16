package wework

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// StreamManager 流式消息管理器
type StreamManager struct {
	streams       map[string]*StreamState
	mutex         sync.RWMutex
	cleanupTicker *time.Ticker
	done          chan bool
}

// StreamState 流式状态
type StreamState struct {
	ID         string
	Content    strings.Builder
	IsActive   bool
	LastUpdate time.Time
	mutex      sync.RWMutex
}

// NewStreamManager 创建流式消息管理器
func NewStreamManager() *StreamManager {
	manager := &StreamManager{
		streams:       make(map[string]*StreamState),
		cleanupTicker: time.NewTicker(5 * time.Minute), // 每5分钟清理一次
		done:          make(chan bool),
	}

	// 启动清理协程
	go manager.cleanupRoutine()

	return manager
}

// Close 关闭流式消息管理器
func (sm *StreamManager) Close() {
	sm.cleanupTicker.Stop()
	close(sm.done)

	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// 清理所有流式状态
	for id := range sm.streams {
		delete(sm.streams, id)
	}
}

// CreateStream 创建新的流式状态
func (sm *StreamManager) CreateStream() (string, error) {
	streamID, err := generateStreamID()
	if err != nil {
		return "", fmt.Errorf("failed to generate stream ID: %w", err)
	}

	state := &StreamState{
		ID:         streamID,
		IsActive:   true,
		LastUpdate: time.Now(),
	}

	sm.mutex.Lock()
	sm.streams[streamID] = state
	sm.mutex.Unlock()

	return streamID, nil
}

// GetStream 获取流式状态
func (sm *StreamManager) GetStream(streamID string) *StreamState {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	state, exists := sm.streams[streamID]
	if !exists {
		return nil
	}

	// 更新最后访问时间
	state.mutex.Lock()
	state.LastUpdate = time.Now()
	state.mutex.Unlock()

	return state
}

// DeleteStream 删除流式状态
func (sm *StreamManager) DeleteStream(streamID string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if state, exists := sm.streams[streamID]; exists {
		state.mutex.Lock()
		state.IsActive = false
		state.mutex.Unlock()
		delete(sm.streams, streamID)
	}
}

// UpdateStreamContent 更新流式内容
func (sm *StreamManager) UpdateStreamContent(streamID, content string, isFinished bool) error {
	sm.mutex.RLock()
	state, exists := sm.streams[streamID]
	sm.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("stream not found: %s", streamID)
	}

	state.mutex.Lock()
	defer state.mutex.Unlock()

	state.Content.Reset()
	state.Content.WriteString(content)
	state.LastUpdate = time.Now()

	if isFinished {
		state.IsActive = false
	}

	return nil
}

// GetStreamContent 获取流式内容
func (state *StreamState) GetStreamContent() (string, bool) {
	if state == nil {
		return "", false
	}

	state.mutex.RLock()
	defer state.mutex.RUnlock()

	return state.Content.String(), state.IsActive
}

// cleanupRoutine 清理过期的流式状态
func (sm *StreamManager) cleanupRoutine() {
	for {
		select {
		case <-sm.cleanupTicker.C:
			sm.cleanupExpiredStreams()
		case <-sm.done:
			return
		}
	}
}

// cleanupExpiredStreams 清理过期的流式状态
func (sm *StreamManager) cleanupExpiredStreams() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	now := time.Now()
	expiredTimeout := 10 * time.Minute // 10分钟超时

	for id, state := range sm.streams {
		state.mutex.RLock()
		isExpired := now.Sub(state.LastUpdate) > expiredTimeout
		isInactive := !state.IsActive
		state.mutex.RUnlock()

		if isExpired || isInactive {
			delete(sm.streams, id)
		}
	}
}

// GetActiveStreamCount 获取活跃流式数量
func (sm *StreamManager) GetActiveStreamCount() int {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	count := 0
	for _, state := range sm.streams {
		state.mutex.RLock()
		if state.IsActive {
			count++
		}
		state.mutex.RUnlock()
	}

	return count
}

// generateStreamID 生成唯一的流式ID
func generateStreamID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("stream_%s_%d", hex.EncodeToString(bytes), time.Now().Unix()), nil
}
