---
name: explore
description: A relentless interview to sharpen a plan, design or idea which also creates durable docs (ADRs, capabilities and glossary) as we go.
disable-model-invocation: true
---

Interview me relentlessly about every aspect of this until we reach a shared understanding. Walk down each branch of the decision tree, resolving dependencies between decisions one-by-one. **For each question, provide your recommended answer**. Use the `domain` skill throughout the session to aid your questions.

*Ask the questions one at a time*, waiting for feedback on each question before continuing. Asking multiple questions at once is bewildering.

Finding _facts_ is your job never the mine. If a fact can be found by exploring the environment (filesystem, tools, codebase etc.), look it up rather than asking me --dispatch a sub-agent to find it when the lookup is slow and keep interviewing while it runs, don't ask me for anything you could lookup yourself. Don't block on it: a running exploration is an unsettled prerequisite so only any question downstream of it wait for the sub-agent to report -- ask the rest addressable questions now. The decisions, though, are mine so put each one to me and wait for an answer.


The session is done when nothing is left unresolved: every branch of the decision tree visited, nothing left silently assumed. Do not decide that yourself, when you see nothing left to ask, say so and wait for me to confirm we have reached a shared understanding.

After confirmation, stop and let the user decide the next step. Suggest running the `propose` skill but never run it yourself.


## Question Format
Follow this format explicitly. Avoid using `AskUserQuestion` or `functions.request_user_input` or any similar
tool. 

```
Q<question number>: <question>

<question context (the "why the question is asked"), brief but clear with just enough detail>

<answer options>

<your recommendation from within the options>
```


