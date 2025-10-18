Short answer: default to not mocking your own code; mock only at boundaries. And spyOn is not a replacement for mocking—it's one kind of mock/stub, useful in narrow cases.

Guidelines:

    What to mock
    • Mock/fake only things at the system edge: network clients, databases, filesystem, time, randomness, process/env, workers/queues, browser APIs.
    • Stub when you must force rare/error paths or prevent slow/side-effects.
    • Prefer contract tests for these boundaries (your code ↔ external service) plus a few "happy path" integration tests with a real instance when feasible.

    What not to mock
    • Don't mock internal modules you own just to "unit-isolate." That couples tests to structure and call patterns, making refactors brittle.
    • For pure, deterministic code you wrote, use the real collaborators and assert on outputs/observable state through public APIs.

    Where spies fit
    • spyOn is still a test double. It can be used in two modes: observe calls while letting the real code run, or replace behavior.
    • Use a spy when the interaction is the behavior you care about (e.g., your service must call the payment gateway exactly once, with specific args; your code must log/emit a metric).
    • Prefer "call-through" spies when safe; stub only to block side effects or force branches.
    • Don't assert on incidental interactions (e.g., how many times a helper function you own was invoked). Assert outcomes instead.

    Anti-patterns
    • Blanket module auto-mocks of your own code.
    • Verifying private/internal call graphs.
    • Using spies to "reach inside" and test implementation details you don't want to promise.
