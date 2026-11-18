import type { ComponentProps } from "react";

// A button in one of the app's styles; native button props, ref included, pass through.
export interface ButtonProps extends ComponentProps<"button"> {
  variant?: "default" | "primary" | "danger";
}

export function Button({ variant = "default", className, ...props }: ButtonProps) {
  // landing: a button style, not a Landing
  const classes = ["btn", variant === "default" ? "" : variant, className ?? ""].filter(Boolean).join(" ");
  return <button {...props} className={classes} />;
}
