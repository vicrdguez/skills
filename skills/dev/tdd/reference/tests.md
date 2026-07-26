# Good and Bad Tests

## Good Tests

**Integration-style**: Test through real interfaces, not mocks of internal parts.

```
GOOD: "user can checkout with valid cart"
  arrange: cart with a product
  act:     checkout(cart, paymentMethod)
  assert:  result.status == "confirmed"
```

Characteristics:

- Tests behavior users/callers care about
- Uses public API only
- Survives internal refactors
- Describes WHAT, not HOW
- One logical assertion per test

## Bad Tests

**Implementation-detail tests**: Coupled to internal structure.

```
BAD: "checkout calls paymentService.process"
  mock paymentService; assert process was called with cart.total
```

Red flags:

- Mocking internal collaborators
- Testing private methods
- Asserting on call counts/order
- Test breaks when refactoring without behavior change
- Test name describes HOW not WHAT
- Verifying through external means instead of interface

An example: 

```typescript
// BAD: Bypasses interface to verify
test("createUser saves to database", async () => {
  await createUser({ name: "Alice" });
  const row = await db.query("SELECT * FROM users WHERE name = ?", ["Alice"]);
  expect(row).toBeDefined();
});

// GOOD: Verifies through interface
test("createUser makes user retrievable", async () => {
  const user = await createUser({ name: "Alice" });
  const retrieved = await getUser(user.id);
  expect(retrieved.name).toBe("Alice");
});
```

**Tautological tests**: Expected value restates the implementation, so the test passes by construction.

```typescript
// BAD: Expected value is recomputed the way the code computes it
test("calculateTotal sums line items", () => {
  const items = [{ price: 10 }, { price: 5 }];
  const expected = items.reduce((sum, i) => sum + i.price, 0);
  expect(calculateTotal(items)).toBe(expected);
});

// GOOD: Expected value is an independent, known literal
test("calculateTotal sums line items", () => {
  expect(calculateTotal([{ price: 10 }, { price: 5 }])).toBe(15);
});
```

## "Through the interface" is recursive — test internals at their own seam

The rule is *don't reach **past** a module's interface into its private internals* — But it is **not** "only
test the topmost public API." A substantial internal module (a slippage model, a state-machine guard)
is itself a module with an interface; write its tests against **that** interface. Use `design` to tell a real internal module (test it) from incidental plumbing (don't pin it).

## Materializing a Gherkin scenario

One scenario → one test, in this project's own framework (ExUnit `test`/`describe`, Jest `it`, Go
table tests…). `Scenario Outline` rows → a parameterized/table-driven test.

| Gherkin | Test |
|---|---|
| `Feature` / `Rule` | a `describe`/`context` group |
| `Background` | shared setup (fixture / `setup` block) |
| `Scenario: <name>` | one test (name it after the scenario) |
| `Given …` (`And`/`But`) | arrange |
| `When …` (`And`/`But`) | act (call the interface) |
| `Then …` (`And`/`But`) | assert on the observable result |
| `Scenario Outline` + `Examples` | one parameterized / table-driven test |

```
Scenario: Reject cancelling a shipped order
  → test "order cancellation — reject cancelling a shipped order"
      arrange: an order in "shipped"
      act:     cancel(order)
      assert:  rejected with "already shipped"; order still "shipped"
```

Name tests after behaviour, not methods. No assertions on log lines or internal call counts.
