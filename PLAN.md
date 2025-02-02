# Forking Implementation Plan

## Current State
- Single PTY per connection
- Shell runs directly in the server process
- No process isolation or forking

## Goals
1. Fork shell processes for better isolation
2. Support proper process cleanup
3. Maintain existing functionality:
   - PTY handling
   - Window size updates
   - Signal handling
   - Session management

## Technical Requirements
(Let's discuss and fill these in) 