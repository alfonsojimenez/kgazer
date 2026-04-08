import { describe, test, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MessageDiff } from "./MessageDiff";
import type { Message } from "@/lib/api";

function makeMessage(overrides: Partial<Message> = {}): Message {
  return {
    id: 1,
    key: "test-key",
    body: { value: "default" },
    partition: 0,
    offset: 100,
    timestamp: "2025-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("MessageDiff", () => {
  test("shows 'No changes' when prev and current bodies are identical", () => {
    const body = { name: "same" };
    const prev = makeMessage({ body, offset: 99 });
    const current = makeMessage({ body, offset: 100 });

    render(<MessageDiff prev={prev} current={current} />);
    expect(screen.getByText("No changes")).toBeInTheDocument();
  });

  test("shows diff with addition/deletion lines when bodies differ", () => {
    const prev = makeMessage({ body: { name: "old" }, offset: 99 });
    const current = makeMessage({ body: { name: "new" }, offset: 100 });

    render(<MessageDiff prev={prev} current={current} />);

    const pre = document.querySelector("pre");
    expect(pre).not.toBeNull();
    expect(pre!.textContent).toContain("-");
    expect(pre!.textContent).toContain("+");
  });

  test("diff is open by default", () => {
    const prev = makeMessage({ body: { a: 1 }, offset: 99 });
    const current = makeMessage({ body: { a: 2 }, offset: 100 });

    render(<MessageDiff prev={prev} current={current} />);
    expect(screen.getByText("Hide diff")).toBeInTheDocument();
  });

  test("shows addition/deletion counts in trigger", () => {
    const prev = makeMessage({ body: { a: 1 }, offset: 99 });
    const current = makeMessage({ body: { a: 2 }, offset: 100 });

    render(<MessageDiff prev={prev} current={current} />);
    const trigger = screen.getByRole("button");
    expect(trigger.textContent).toMatch(/\+\d+/);
    expect(trigger.textContent).toMatch(/-\d+/);
  });

  test("toggle hides and shows diff content", async () => {
    const user = userEvent.setup();
    const prev = makeMessage({ body: { a: 1 }, offset: 99 });
    const current = makeMessage({ body: { a: 2 }, offset: 100 });

    render(<MessageDiff prev={prev} current={current} />);
    expect(document.querySelector("pre")).toBeInTheDocument();

    await user.click(screen.getByText("Hide diff"));
    expect(screen.getByText("Show diff")).toBeInTheDocument();
  });
});
