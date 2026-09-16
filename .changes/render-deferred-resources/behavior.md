# Render Typed Deferred Resources Behavior

## Feature: Discover and render typed deferred resources

### Background

- Given the versioned embedded Skill Definitions and resources are served through the existing in-process `skl` CLI
- And named-resource calls have no Workflow Backend access and perform no Git inspection or filesystem mutation
- And tests use independent expected instructions and argument values, not the authored template source as their oracle

### Scenario Outline: B1 Discover inputs without loading the procedure

Maps to DOD1.

- Given the requested resource is <resource> owned by <owner>
- When I run `skl skill --resource <resource> --describe-inputs <owner>` without input values
- Then the command succeeds and describes every accepted input's name, meaning, scalar type and any allowed choices, and required status
- And the description matches the inputs accepted by retrieval, or explicitly reports that the resource has no inputs
- But no procedural resource content is rendered

Examples:

| owner | resource |
| implement | reference/submission.md |
| implement | reference/decision.md |
| watchdog | reference/review.md |
| tdd | reference/tests.md |

### Scenario Outline: B2 Render the applicable Implement submission procedure

Maps to DOD2.

- Given the invocation establishes <procedure> and a concrete private Result Document directory
- When I retrieve Implement's `reference/submission.md` with its accepted typed inputs
- Then the instructions name the bound `submission.md` destination and cover Summary, Verification, and the current Audit ledger
- And <finding-guidance>
- And scenario-to-test evidence, Full Gate results, Audit dispositions, and the actually audited fixed point and head remain worker-authored content
- And the instructions retain publication as opaque prose with the engine's machine-owned closing footer, not as parsed rendering inputs

Examples:

| procedure | finding-guidance |
| First implementation | No finding-driven Rework section or resubmission instructions are included |
| Finding-driven Rework | Every existing finding is mapped by stable ID to its resolution commit and evidence or linked Debt Marker, with the Audit ledger current |

### Scenario Outline: B3 Render a decision with the applicable preservation instructions

Maps to DOD3 and DOD5.

- Given the worker has reached the Needs Human step and established whether implementation work needs preservation
- When I retrieve Implement's `reference/decision.md` with a bound private directory and explicit preservation value <value>
- Then the instructions name the bound `decision.md` destination and request the blocking requirement or rule, current state, completed work, options, consequences, and recommendation
- And <preservation>
- And the permitted decision reason remains carried separately by the semantic Needs Human command, not parsed from the opaque prose
- And the instructions do not invent Completion, tick unfinished work, or retire an incomplete ledger during this pause

Examples:

| value | preservation |
| false | Retrieval succeeds with the required boolean present and does not direct creation of a draft Submission to preserve nonexistent work |
| true | The worker is directed to push existing work and supply the applicable `submission.md` from the same private directory as `--body` to preserve one draft Submission |

### Scenario Outline: B4 Retrieve review guidance before deciding dispositions

Maps to DOD4.

- Given Watchdog has established <review-context>, a private Result Document directory, and the original full reviewed-head SHA
- When I retrieve `reference/review.md` with the established review inputs but no verdict, finding dispositions, or Result Document prose
- Then the complete review resource binds the supplied round, original reviewed head, and document destinations
- And `summary.md` must retain every existing finding's Submission-local monotonic `W<n>` identity, including when the retained Review Count has reset
- And only directives from an owner, member, or collaborator posted after the finding are eligible; trimming and case-insensitive interpretation apply, and the latest authorized directive wins
- And `WAIVE` accepts without debt while `BLOCK` and `NOTE` retain their dispositions, with no inferred directive from reactions, silence, or deleted comments
- And opaque summaries and finding bodies remain separate from optional structured anchors and the semantic verdict flag
- And conditional pass instructions preserve Audit and review evidence and copy historical Manual Verification verbatim with unchecked boxes
- But no disposition is required before the authorization and precedence guidance is available, and a later Debt Marker head does not replace the original reviewed head

Examples:

| review-context |
| Round 1 with no prior findings |
| Round 2 with existing findings and human directives |
| Round 1 after checkpoint loss with existing findings and human directives still present |

### Scenario Outline: B5 Reject invalid resource inputs before rendering

Maps to DOD5.

- Given a real converted resource and otherwise valid inputs
- When I request retrieval with <invalid-input>
- Then the CLI returns an error identifying <repair-information>
- And no procedural content or generic fallback is returned, no Workflow Backend is called, and no file or Workflow State is changed

Examples:

| invalid-input | repair-information |
| An input without the first `=` separator | The required `name=value` syntax |
| An undeclared input name | The unknown name and how to discover accepted inputs |
| The same declared input supplied twice | The duplicate name, even if both values are identical |
| A required input omitted | The missing name, regardless of its scalar zero value or default |
| An unparseable declared boolean or integer | The input name and expected scalar type |
| An unsupported procedure choice | The input name and supported choices |
| A zero review round | The input name and positive-round constraint |
| `--input` or `--describe-inputs` used without `--resource` | The required named-resource context |

### Scenario: B6 Preserve literal input data across stateless calls

Maps to DOD6.

- Given a valid string input for a real resource contains spaces, a comma, embedded equals, single and double quotes, and literal Go template syntax
- When I retrieve the resource through repeated `--input name=value` arguments
- Then splitting occurs only at the first `=` and the rendered value preserves every supplied character, including leading or trailing spaces where valid for that input
- And supplied template syntax remains literal data rather than executing or injecting variables
- When I retrieve the resource again with different values and then omit a required value
- Then the second rendering uses only its own inputs and the final call rejects the omission rather than reusing prior values
- And this flag handling leaves existing non-resource slice-flag splitting and trimming behavior unchanged

## Feature: Deliver valid deferred commands from real parent instructions

### Scenario Outline: B7 Follow a concrete parent's deferred resource command

Maps to DOD6 and DOD7.

- Given a concrete <parent> invocation with its engine-established procedure and references
- And its applicable bound string values exercise spaces, commas, equals, and shell quotes
- When I read the parent instructions at <step>
- Then they supply the resource command with all accepted invocation-known values bound literally and correctly shell-quoted
- And only <later-values> remain to be established, with their meanings, allowed values, and acquisition step explained
- And resource content remains deferred, without new Backend queries, Git inspection, session storage, or prerendered files solely for specialization
- When I fill only those later-established values and run the emitted command through the public resource CLI
- Then it succeeds with the applicable complete real resource and the exact original values, without further reconstruction of settled inputs

Examples:

| parent | step | later-values |
| First Implement invocation | Writing the submission after implementation and Audit | None |
| Finding-driven Rework invocation | Writing the updated submission after resolving findings | None |
| Implement invocation where preservation is not yet known | Preparing a Needs Human decision | Whether work now needs preservation |
| Watchdog invocation with established round and original reviewed head | Before assigning finding dispositions | None; verdict and finding choices are not inputs needed here |

### Scenario: B8 Keep context-free retrieval and bundled ownership usable

Maps to DOD7 and DOD8.

- Given plain context-free resources and guaranteed included Skill Definitions still use the embedded distribution
- When I retrieve a no-facts Implement or Watchdog definition and the bundled Implement definitions through the existing CLI
- Then converted resource pointers route to the concrete invocation's command or valid input discovery rather than advertise invalid bare calls
- And guaranteed included definitions appear once, without disclosing deferred resource bodies
- And references in included definitions retain their owning skill rather than becoming resources of the including skill
- When I retrieve existing context-free references, including Propose's current artifact formats, with no inputs
- Then their content and owner-relative retrieval remain usable without requiring invocation state
- And existing Markdown/JSON selection and lifecycle output defaults are unchanged

### Scenario: B9 Expose only public resources from the embedded distribution

Maps to DOD9.

- Given authored private Skill Modules are embedded alongside public definitions and resources
- When I inspect a skill's public resource list and request a private module by its owner-relative name, both for retrieval and input discovery
- Then the module is absent from the list and both requests fail as unavailable resources without disclosing its contents
- And `SKILL.md` remains a definition, not a public resource
- And a public resource remains retrievable only under its owning skill, including when that skill is bundled elsewhere
