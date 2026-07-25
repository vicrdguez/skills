# {Change title} Behavior

<!--
Specify behavior in Gherkin NOTATION (there are no .feature files and no Cucumber and no runtime).

Use the keywords that make behavior unambiguous and map cleanly onto a test;
skip the ones that only exist to drive the Cucumber engine.

Allowed keywords:
  - Feature: a capability or behavior under change
  - Rule (optional): a business rule / invariant; group its scenarios beneath it
  - Scenario: one concrete behavior  ->  becomes one test
  - Given / When / Then: arrange / act / assert
  - And / But: continue the previous Given/When/Then, readably
  - Background (optional): shared Given steps for a Feature/Rule  ->  shared test setup
  - Scenario Outline + Examples:   table-driven cases  ->  one parameterized test data tables (|), doc strings  when a step needs structured data

Skip:
  *        the anonymous step bullet — it loses the Given/When/Then -> arrange/act/assert mapping
  @tags    no runtime to filter on; tasks.md already carries traceability

Rules:
- Each Scenario is observable through a module's interface, not incidental internal state.
- One behavior per Scenario.
- Reach for Rule when a change turns on an invariant.
- Give a critical INTERNAL module its own Feature — those scenarios become tests at that module's seam.
Delete this comment in the real file.
-->


## Feature: {capability or behavior under change}

#### Scenario: {clear, specific name — becomes the test name}
- Given {the starting state}
- When {the action taken through a public interface}
- Then {the observable outcome}
- And {a further observable outcome}

---

### Example

```
## Feature: Order cancellation

  ### Background:
  - Given a customer with a placed order

  ### Rule: An order can only be cancelled before it ships

    #### Scenario: Cancel an unshipped order
    - When the customer cancels the order
    - Then the order moves to state "cancelled"
    - And a full refund is initiated

    #### Scenario: Cannot cancel a shipped order
    - Given the order has shipped
    - When the customer attempts to cancel the order
    - Then the cancellation is rejected with reason "already shipped"
    - But the order remains in state "shipped"

    #### Scenario Outline: Refund equals the order total
    - When the customer cancels the order totalling <total>
    - Then a refund of <total> is initiated

    Examples:
    | total |
    | 1000  |
    | 4250  |

```

A critical internal module — tested at its own seam, not via the public API

```
## Feature: Order state-machine guard

  ### Rule: Only placed -> cancelled is a legal transition

    #### Scenario: Reject a cancel from shipped
      - Given a state "shipped"
      - When the guard evaluates a "cancel" transition
      - Then the transition is rejected as illegal
```
