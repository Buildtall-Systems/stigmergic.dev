# Fix Plan: fsnotify and Graceful Shutdown

## Issues Identified

### fsnotify Event Detection
- watcher.go:256-275: Event conversion uses if/else chain, only captures one flag but fsnotify events can have multiple flags (CREATE|WRITE)
- New file detection unreliable: editors often WRITE instead of CREATE
- Race condition: new directories might get files before watch is added

### Graceful Shutdown
- server.go:93-120: Start() returns immediately when server succeeds (errChan gets nil), never waits for signal
- watcher.go:177-187: Close() doesn't wait for eventLoop goroutine to finish
- server.go:135-188: broadcastEvents() has no WaitGroup coordination
- watcher.go:246-253: Debounce timer can send to closed Events channel after Close()

### Concurrency
- No WaitGroup tracking for goroutines
- No context propagation for coordinated cancellation
- Channels closed before goroutines finish

## Fix Strategy

### Phase 1: Fix Event Detection
1. Change event conversion to check all flags with Has(), not if/else
2. Treat WRITE on non-existent file as CREATE
3. Add stat check in event handler for new files

### Phase 2: Fix Graceful Shutdown
1. Add sync.WaitGroup to Watcher for eventLoop
2. Add context.Context to Server and Watcher
3. Fix shutdown sequence:
   - Close done channel / cancel context
   - Wait for WaitGroup
   - Close fsnotify watcher
   - Close output channels
4. Fix debounce: check done channel before sending to Events
5. Fix server Start(): only return on error or after shutdown completes

### Phase 3: Test
1. Build after each change
2. Run full test suite after each phase
3. Manual test: create files, new directories, graceful shutdown (Ctrl+C)
