# Agentengine Development Rules

This project follows HOP (Human-Oriented Programming).

- Keep the complete cognition loop visible in `agent/agent.go`.
- Prefer fewer concepts, files, interfaces, and configuration options.
- Organize code by the business subject: agent (loop, workflow, task), model connection, session.
- Keep related behavior together. Split a file only when different parts clearly change for different reasons.
- Name methods for what they do without repeating package or type context.
- Comment business intent and non-obvious decisions, not Go syntax.
- This is a library. Applications supply the system prompt and the toolbox.
- Do not add speculative AGI infrastructure. Add memory, planning, reflection, or multi-agent work only with a concrete usable workflow.
