import { useState } from "react";

export default function App() {
  const [msg] = useState("React");
  return <h1 style={{ fontSize: 12 }}>Hello {msg}!</h1>;
}
