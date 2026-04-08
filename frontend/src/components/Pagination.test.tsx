import { describe, test, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Pagination } from "./Pagination";

describe("Pagination", () => {
  test("returns null when only one page", () => {
    const { container } = render(
      <Pagination page={1} total={10} limit={25} onPageChange={() => {}} />
    );
    expect(container.innerHTML).toBe("");
  });

  test("renders page info and navigation buttons", () => {
    render(<Pagination page={2} total={75} limit={25} onPageChange={() => {}} />);

    expect(screen.getByText("75 results")).toBeInTheDocument();
    expect(screen.getByText("2 / 3")).toBeInTheDocument();
  });

  test("disables previous button on first page", () => {
    render(<Pagination page={1} total={50} limit={25} onPageChange={() => {}} />);

    const buttons = screen.getAllByRole("button");
    expect(buttons[0]).toBeDisabled();
    expect(buttons[1]).not.toBeDisabled();
  });

  test("disables next button on last page", () => {
    render(<Pagination page={2} total={50} limit={25} onPageChange={() => {}} />);

    const buttons = screen.getAllByRole("button");
    expect(buttons[0]).not.toBeDisabled();
    expect(buttons[1]).toBeDisabled();
  });

  test("calls onPageChange with correct page on next click", async () => {
    const user = userEvent.setup();
    const onPageChange = vi.fn();
    render(<Pagination page={1} total={50} limit={25} onPageChange={onPageChange} />);

    const buttons = screen.getAllByRole("button");
    await user.click(buttons[1]);
    expect(onPageChange).toHaveBeenCalledWith(2);
  });

  test("calls onPageChange with correct page on previous click", async () => {
    const user = userEvent.setup();
    const onPageChange = vi.fn();
    render(<Pagination page={2} total={50} limit={25} onPageChange={onPageChange} />);

    const buttons = screen.getAllByRole("button");
    await user.click(buttons[0]);
    expect(onPageChange).toHaveBeenCalledWith(1);
  });

  test("shows singular 'result' for total of 1", () => {
    render(<Pagination page={1} total={26} limit={25} onPageChange={() => {}} />);
    expect(screen.getByText("26 results")).toBeInTheDocument();
  });
});
