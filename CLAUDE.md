# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## CRITICAL RULES - READ FIRST

### NO COMMENTS RULE
- **NEVER write comments on the same line as code**
- **NEVER write comments at all**
- **If you write comments, you'll go in the box**
- **NO COMMENTS!!!!**

### Context Definition
The term "context" and the "gathering context" procedure are defined at `ai/context.md`. Always gather context before writing code to ensure you're working with current, accurate documentation rather than potentially outdated training data.

### MANDATORY SKILLS INVOCATION RULE
- **ALWAYS invoke the `go-developer` skill at the start of any session involving Go code**
- **ALWAYS invoke the `buildtall-go` skill at the start of any session working on this project**
- **ALWAYS invoke the `context-gathering` skill before gathering context for any library or API**
- **These skills provide critical domain knowledge and conventions that override any default behavior**
- **Invoke ALL THREE skills together at session start when working on Go code in this repository**

### MANDATORY TESTING RULE
- **ALWAYS run the full test suite after making changes to code.
- **ALL tests MUST pass before considering work complete**
- **NEVER submit incomplete work with failing tests**
- **If tests fail, you MUST fix them immediately**
- **Test failures are considered blocking issues**
- **This is NOT optional - testing is mandatory for every change**
- **NEW FEATURES REQUIRE NEW TESTS:** Every new component, function, or feature MUST have corresponding tests written**

### GIT OPERATIONS RULE
- **NEVER make git commits, git add, or any git staging operations**
- **ONLY the operator (user) is allowed to commit code**

### SUBAGENT RESTRICTION ENFORCEMENT RULE
- **ALL user/operator restrictions apply transitively to ANY subagents, tools, or autonomous agents**
- **When user says "edit no code" or "write no code", NO subagent may edit or write code**
- **When user says "read only" or "investigate only", ALL subagents must be constrained to read-only operations**
- **NEVER delegate tasks to subagents that would violate explicit user restrictions**
- **If using Task tool when user has imposed restrictions, MUST explicitly constrain the subagent with phrases like "DO NOT edit any files", "DO NOT write any code", "ONLY read and analyze"**
- **Subagent violations of user restrictions are treated as direct violations by the primary agent**
- **When in doubt about subagent scope, ask user for explicit permission before delegating**

### TOOL USAGE RULE
- **ALWAYS use built-in tools when available instead of command line alternatives**
- **Use Glob tool for listing files/directories instead of `ls` command**
- **Use Read tool for reading files instead of `cat` command**
- **Use Grep tool for searching files instead of `grep` command**
- **Use Edit/Write tools for file modifications instead of `sed` or other commands**
- **Only use Bash tool when there is NO built-in alternative available**

### SCOPE LIMITATION RULE
- **ONLY do what the user/operator explicitly asks for**
- **NEVER add extra features, improvements, or changes beyond the specific request**
- **NEVER make assumptions about what the user might want**
- **If asked to do X, do ONLY X - nothing more, nothing less**
- **Do not proactively fix unrelated issues unless specifically asked**

### Rules We Found From Continued Interaction

- you should always provide a one line description of why you are using a tool, BEFORE making the call. otherwise your call will be interrupted
- build and test: when implementing new features you write the code for the new feature, add a test or modify an existing test if refactoring, build the app, then run the full test suite
- never mark code editing tasks as complete without having run a build and full test suite
- translate "AAH" to "ANSWER AND HALT"
- translate "MNE" into "MAKE NO EDITS"
- translate "RSNGC" to "READ SOURCE CODE AND GATHER CONTEXT"
- I always want you to tell me don't know when you don't know
- NEVER EVER SAY "You're absolutely right" -- NEVER NEVER NEVER
- answers from memory are no different than lies
- you must use Makefile rules to build, test, lint, etc when available. when not available, add them.

### Supplimentary Rules

Please find a list of supplimentary rules at ai/rules.md

## Project Overview

**Project:** stigmergic.dev
**Type:** project
**Description:** a way to watch and read markdown written to the filesystem in a dynamic way
**Tech Stack:** go, goldmark markdown parsing, http

## Development Commands

### Testing

## Architecture

### Data Flow

### Key Components

### Database Schema

## Development Guidelines

### Code Style & UI Components

### Styling Rules (IMPORTANT)

### Design System

### Accessibility & Performance

### Security
- you are NEVER allowed to write to /tmp