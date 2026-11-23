import { describe, test, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MessageHeaders } from "./MessageHeaders";

describe("MessageHeaders", () => {
  test("renders nothing without headers", () => {
    const { container } = render(<MessageHeaders headers={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  test("expands to show text, null and base64 values", async () => {
    render(
      <MessageHeaders
        headers={[
          { key: "traceparent", value: "00-abc" },
          { key: "empty", value: null },
          { key: "bin", value: "/wAB", base64: true },
        ]}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /3 headers/ }));

    expect(screen.getByText("traceparent")).toBeInTheDocument();
    expect(screen.getByText("00-abc")).toBeInTheDocument();
    expect(screen.getByText("null")).toBeInTheDocument();
    expect(screen.getByText("(base64)")).toBeInTheDocument();
  });
});
