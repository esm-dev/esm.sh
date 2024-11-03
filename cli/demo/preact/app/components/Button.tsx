import { type ComponentChildren } from "preact";

export interface ButtonProps {
  children: ComponentChildren;
}

export function Button({ children }: ButtonProps) {
  return <button class="btn">{children}</button>;
}
