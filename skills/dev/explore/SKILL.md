---
name: explore
description: A relentless interview to sharpen a plan, design or idea which also creates durable docs (ADRs, capabilities and glossary) as we go.
disable-model-invocation: true
---

Interview me relentlessly about every aspect of this until we reach a shared understanding. Walk down each branch of the decision tree, resolving dependencies between decisions one-by-one. **For each question, provide your recommended answer**.

*Ask the questions one at a time*, waiting for feedback on each question before continuing. Asking multiple questions at once is bewildering.

If a fact can be found by exploring the environment (filesystem, tools, codebase etc.), look it up rather than asking me. The decisions, though, are mine — put each one to me and wait for my answer.

Do not act on it until I confirm we have reached a shared understanding.

Use `/domain` skill throughout the session.


## Question Format
Follow this format explicitly. Avoid using `AskUserQuestion` or `functions.request_user_input` or any similar
tool. 

```
Q<question number>: <question>

<question context (the "why the question is asked"), brief but clear with just enough detail>

<answer options>

<your recommendation from within the options>
```


