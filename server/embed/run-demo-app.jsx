import { useEffect } from "preact/hooks";
import confetti from "canvas-confetti";

export function App() {
  useEffect(() => {
    confetti();
  }, []);
  return <h1>Hello world!</h1>;
}
