# Code Review with MPT - Guide and Template

## Running Code Reviews

### Quick Start

```bash
# Save changes to a file
git diff > /tmp/changes.diff

# Run review with file input
mpt -f changes.diff --openai.enabled --google.enabled --anthropic.enabled --timeout=5m -p "Perform a comprehensive code review of these changes. Analyze the design patterns and architecture. Identify any security vulnerabilities or risks. Evaluate code readability, maintainability, and idiomatic usage. Suggest specific improvements where needed." && rm -fv /tmp/changes.diff
```

### Important Note

Always use the file input method with the `-f` flag. Directly piping content from git diff does not work reliably with MPT and Claude Code.

### Review Process

1. **Generate reviews** using one of the commands above
2. **Merge insights** from all providers, focusing on the most important issues first
3. **Prioritize findings** in this order:
   - Security vulnerabilities
   - Design/architectural issues
   - Performance concerns
   - Maintainability/readability issues
   - Style/formatting issues
4. **Implement changes** based on the review feedback

### Tips for Better Reviews

- Include a specific prompt to focus the review on your areas of concern
- Use a reasonable timeout (3-5 minutes) to allow thorough analysis
- Enable multiple providers for diverse perspectives
- For large diffs, consider breaking them into smaller chunks or reviewing specific files

### Example Review Workflow

1. Make your code changes and stage them
2. Generate a diff file: `git diff > review.diff`
3. Run MPT against the diff: `mpt -f review.diff --openai.enabled --google.enabled --anthropic.enabled --timeout=5m -p "Perform a focused code review on security issues and design patterns. Identify vulnerabilities, assess architectural decisions, and suggest improvements."`
4. Consolidate feedback from all providers
5. Make improvements based on the feedback
6. Re-run the review if necessary

### Common Review Prompts

Here are some useful prompts for different types of reviews:

#### General Code Review
```
Perform a comprehensive code review analyzing design, security, readability, maintainability, and idiomatic code usage. Identify issues and suggest specific improvements.
```

#### Security-Focused Review
```
Review code changes with a focus on security vulnerabilities, potential exploits, input validation, authentication, authorization, and secure coding practices. Highlight any risks and provide mitigation strategies.
```

#### Performance Review
```
Analyze code for performance issues including inefficient algorithms, memory leaks, unnecessary computations, bottlenecks, and resource usage. Suggest optimization opportunities with specific examples.
```

#### Design Pattern Review
```
Evaluate code against SOLID principles, appropriate design patterns, and architecture best practices. Identify areas where patterns could improve maintainability, flexibility, and scalability.
```

---

## Code Review Template

### Overview

*Brief summary of the changes being reviewed and their purpose*

### Critical Issues

*Issues that must be addressed before merging*

| Issue | Location | Description | Recommendation |
|-------|----------|-------------|----------------|
|       |          |             |                |
|       |          |             |                |

### Security Concerns

*Potential security vulnerabilities or risks*

| Issue | Location | Description | Recommendation |
|-------|----------|-------------|----------------|
|       |          |             |                |
|       |          |             |                |

### Design and Architecture

*Feedback on design patterns, SOLID principles, and overall architecture*

| Issue | Location | Description | Recommendation |
|-------|----------|-------------|----------------|
|       |          |             |                |
|       |          |             |                |

### Code Quality

*Feedback on readability, maintainability, and idiomatic code practices*

| Issue | Location | Description | Recommendation |
|-------|----------|-------------|----------------|
|       |          |             |                |
|       |          |             |                |

### Performance Considerations

*Potential performance issues or optimizations*

| Issue | Location | Description | Recommendation |
|-------|----------|-------------|----------------|
|       |          |             |                |
|       |          |             |                |

### Testing

*Feedback on test coverage and test quality*

| Issue | Location | Description | Recommendation |
|-------|----------|-------------|----------------|
|       |          |             |                |
|       |          |             |                |

### Positive Highlights

*Notable strengths and good practices in the code*

- 
- 
- 

### Proposed Changes

*Specific code changes recommended to address issues*

#### 1. Issue: *brief description*

**Location:** *file and line number*

**Current code:**
```go
// Paste the current code here
```

**Proposed change:**
```go
// Paste the proposed improved code here
```

**Rationale:** *explanation of why this change is recommended*

#### 2. Issue: *brief description*

**Location:** *file and line number*

**Current code:**
```go
// Paste the current code here
```

**Proposed change:**
```go
// Paste the proposed improved code here
```

**Rationale:** *explanation of why this change is recommended*

### Summary

*Overall assessment and recommendations*