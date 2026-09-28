import { describe, test, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FieldFilterPopover } from "./FieldFilterPopover";

const sampleFields = [
  "metadata.source",
  "ownerId",
  "ownerType",
  "periodId",
  "spend",
  "spendBreakdown[].amount",
  "spendBreakdown[].type",
];

describe("FieldFilterPopover", () => {
  test("renders trigger button", () => {
    render(
      <FieldFilterPopover
        fields={sampleFields}
        activeFilters={[]}
        onAddFilter={() => {}}
        onRemoveFilter={() => {}}
      />,
    );
    expect(screen.getByText("Add Filter")).toBeInTheDocument();
  });

  test("shows filter count on trigger", () => {
    render(
      <FieldFilterPopover
        fields={sampleFields}
        activeFilters={[{ field: "ownerId", value: "2101498" }]}
        onAddFilter={() => {}}
        onRemoveFilter={() => {}}
      />,
    );
    expect(screen.getByText("Filters (1)")).toBeInTheDocument();
  });

  test("opens popover and shows top-level leaf fields", async () => {
    const user = userEvent.setup();
    render(
      <FieldFilterPopover
        fields={sampleFields}
        activeFilters={[]}
        onAddFilter={() => {}}
        onRemoveFilter={() => {}}
      />,
    );
    await user.click(screen.getByText("Add Filter"));
    expect(screen.getByText("ownerId")).toBeInTheDocument();
    expect(screen.getByText("ownerType")).toBeInTheDocument();
    expect(screen.getByText("periodId")).toBeInTheDocument();
    expect(screen.getByText("spend")).toBeInTheDocument();
  });

  test("shows collapsed branches with array badge", async () => {
    const user = userEvent.setup();
    render(
      <FieldFilterPopover
        fields={sampleFields}
        activeFilters={[]}
        onAddFilter={() => {}}
        onRemoveFilter={() => {}}
      />,
    );
    await user.click(screen.getByText("Add Filter"));
    expect(screen.getByText("spendBreakdown[]")).toBeInTheDocument();
    expect(screen.getByText("array")).toBeInTheDocument();
  });

  test("expands branch to show nested fields", async () => {
    const user = userEvent.setup();
    render(
      <FieldFilterPopover
        fields={sampleFields}
        activeFilters={[]}
        onAddFilter={() => {}}
        onRemoveFilter={() => {}}
      />,
    );
    await user.click(screen.getByText("Add Filter"));
    await user.click(screen.getByText("spendBreakdown[]"));
    expect(screen.getAllByText("type")).toHaveLength(1);
    expect(screen.getAllByText("amount")).toHaveLength(1);
  });

  test("selecting a leaf enables value input", async () => {
    const user = userEvent.setup();
    render(
      <FieldFilterPopover
        fields={sampleFields}
        activeFilters={[]}
        onAddFilter={() => {}}
        onRemoveFilter={() => {}}
      />,
    );
    await user.click(screen.getByText("Add Filter"));
    await user.click(screen.getByText("ownerId"));
    const input = screen.getByPlaceholderText("ownerId:");
    expect(input).not.toBeDisabled();
  });

  test("adding filter calls onAddFilter with correct path and value", async () => {
    const user = userEvent.setup();
    const onAddFilter = vi.fn();
    render(
      <FieldFilterPopover
        fields={sampleFields}
        activeFilters={[]}
        onAddFilter={onAddFilter}
        onRemoveFilter={() => {}}
      />,
    );
    await user.click(screen.getByText("Add Filter"));
    await user.click(screen.getByText("spendBreakdown[]"));
    await user.click(screen.getAllByText("type")[0]);
    const input = screen.getByPlaceholderText("type:");
    fireEvent.change(input, { target: { value: "ca:click" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onAddFilter).toHaveBeenCalledWith("spendBreakdown[].type", "ca:click");
  });

  test("shows active filters inside popover", async () => {
    const user = userEvent.setup();
    render(
      <FieldFilterPopover
        fields={sampleFields}
        activeFilters={[{ field: "ownerId", value: "2101498" }]}
        onAddFilter={() => {}}
        onRemoveFilter={() => {}}
      />,
    );
    await user.click(screen.getByText("Filters (1)"));
    expect(screen.getByText("2101498")).toBeInTheDocument();
  });

  test("removing a filter calls onRemoveFilter", async () => {
    const user = userEvent.setup();
    const onRemoveFilter = vi.fn();
    render(
      <FieldFilterPopover
        fields={sampleFields}
        activeFilters={[{ field: "ownerId", value: "2101498" }]}
        onAddFilter={() => {}}
        onRemoveFilter={onRemoveFilter}
      />,
    );
    await user.click(screen.getByText("Filters (1)"));
    const removeBtn = screen.getAllByRole("button").find((btn) =>
      btn.querySelector(".lucide-x"),
    );
    expect(removeBtn).toBeDefined();
    await user.click(removeBtn!);
    expect(onRemoveFilter).toHaveBeenCalledWith("ownerId");
  });

  test("add button disabled without selected field", async () => {
    const user = userEvent.setup();
    render(
      <FieldFilterPopover
        fields={sampleFields}
        activeFilters={[]}
        onAddFilter={() => {}}
        onRemoveFilter={() => {}}
      />,
    );
    await user.click(screen.getByText("Add Filter"));
    const input = screen.getByPlaceholderText("Select a field…");
    expect(input).toBeDisabled();
  });

  test("shows no fields message when fields empty", async () => {
    const user = userEvent.setup();
    render(
      <FieldFilterPopover
        fields={[]}
        activeFilters={[]}
        onAddFilter={() => {}}
        onRemoveFilter={() => {}}
      />,
    );
    await user.click(screen.getByText("Add Filter"));
    expect(screen.getByText("No fields available")).toBeInTheDocument();
  });

  test("search filter narrows tree to matching branches", async () => {
    const user = userEvent.setup();
    render(
      <FieldFilterPopover
        fields={sampleFields}
        activeFilters={[]}
        onAddFilter={() => {}}
        onRemoveFilter={() => {}}
      />,
    );
    await user.click(screen.getByText("Add Filter"));
    const searchInput = screen.getByPlaceholderText("Search fields…");
    await user.type(searchInput, "spend");
    expect(screen.getByText("spend")).toBeInTheDocument();
    expect(screen.getByText("spendBreakdown[]")).toBeInTheDocument();
    expect(screen.queryByText("ownerId")).not.toBeInTheDocument();
    expect(screen.queryByText("ownerType")).not.toBeInTheDocument();
  });

  test("search filter auto-expands matching branches", async () => {
    const user = userEvent.setup();
    render(
      <FieldFilterPopover
        fields={sampleFields}
        activeFilters={[]}
        onAddFilter={() => {}}
        onRemoveFilter={() => {}}
      />,
    );
    await user.click(screen.getByText("Add Filter"));
    const searchInput = screen.getByPlaceholderText("Search fields…");
    await user.type(searchInput, "type");
    expect(screen.getAllByText("type")).toHaveLength(1);
    expect(screen.getByText("spendBreakdown[]")).toBeInTheDocument();
    expect(screen.queryByText("amount")).not.toBeInTheDocument();
    expect(screen.queryByText("ownerId")).not.toBeInTheDocument();
  });

  test("search filter shows no matching message", async () => {
    const user = userEvent.setup();
    render(
      <FieldFilterPopover
        fields={sampleFields}
        activeFilters={[]}
        onAddFilter={() => {}}
        onRemoveFilter={() => {}}
      />,
    );
    await user.click(screen.getByText("Add Filter"));
    const searchInput = screen.getByPlaceholderText("Search fields…");
    await user.type(searchInput, "nonexistent");
    expect(screen.getByText("No matching fields")).toBeInTheDocument();
  });

  test("clearing search restores full tree", async () => {
    const user = userEvent.setup();
    render(
      <FieldFilterPopover
        fields={sampleFields}
        activeFilters={[]}
        onAddFilter={() => {}}
        onRemoveFilter={() => {}}
      />,
    );
    await user.click(screen.getByText("Add Filter"));
    const searchInput = screen.getByPlaceholderText("Search fields…");
    await user.type(searchInput, "spend");
    expect(screen.queryByText("ownerId")).not.toBeInTheDocument();
    await user.clear(searchInput);
    expect(screen.getByText("ownerId")).toBeInTheDocument();
  });

  test("value placeholder shows last segment not full path", async () => {
    const user = userEvent.setup();
    render(
      <FieldFilterPopover
        fields={sampleFields}
        activeFilters={[]}
        onAddFilter={() => {}}
        onRemoveFilter={() => {}}
      />,
    );
    await user.click(screen.getByText("Add Filter"));
    await user.click(screen.getByText("spendBreakdown[]"));
    await user.click(screen.getAllByText("type")[0]);
    expect(screen.getByPlaceholderText("type:")).toBeInTheDocument();
  });
});
