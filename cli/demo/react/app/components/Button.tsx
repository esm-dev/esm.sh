import { type PropsWithChildren } from "react";

export function Button({ children }: PropsWithChildren<{}>) {
  return <button className="btn">{children}</button>;
}
