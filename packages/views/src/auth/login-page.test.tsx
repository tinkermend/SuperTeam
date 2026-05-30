import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LoginPage } from "./login-page";

afterEach(() => {
  cleanup();
});

describe("LoginPage", () => {
  it("submits entered credentials", () => {
    const onSubmit = vi.fn();

    render(<LoginPage error={null} isPending={false} onSubmit={onSubmit} />);

    fireEvent.change(screen.getByLabelText("账号"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("密码"), { target: { value: "secret" } });
    fireEvent.click(screen.getByRole("button", { name: "登录" }));

    expect(onSubmit).toHaveBeenCalledWith({
      username: "admin",
      password: "secret",
    });
  });

  it("disables the submit button while pending", () => {
    render(<LoginPage error={null} isPending={true} onSubmit={vi.fn()} />);

    expect(screen.getByRole("button", { name: "登录中..." })).toBeDisabled();
  });

  it("does not submit again while a login request is pending", () => {
    const onSubmit = vi.fn();

    render(<LoginPage error={null} isPending={true} onSubmit={onSubmit} />);

    fireEvent.submit(screen.getByRole("form", { name: "登录表单" }));

    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("renders an authentication error", () => {
    render(<LoginPage error="用户名或密码错误" isPending={false} onSubmit={vi.fn()} />);

    expect(screen.getByRole("alert")).toHaveTextContent("用户名或密码错误");
  });
});
