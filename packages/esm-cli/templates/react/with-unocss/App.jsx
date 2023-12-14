import { useState } from "react";

const externalLinks = [
  ["Get Started", "https://esm.sh/hot/docs/get-started"],
  ["Docs", "https://esm.sh/hot/docs"],
  ["Github", "https://github.com/esm-dev/esm.sh"],
];

export default function App() {
  const [count, setCount] = useState(0);

  return (
    <div className="w-screen h-screen flex flex-col items-center justify-center">
      <p className="logo">
        <img className="w-18 h-18" src="/assets/logo.svg" title="Hot" />
      </p>
      <h1 className="flex items-center gap-3 text-3xl mt-6 font-bold">
        esm.sh/hot
        <svg
          className="w-4.5 h-4.5"
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 14 14"
        >
          <path
            fill="#999999"
            d="m5.267 4.792l1.864 6.203a1.5 1.5 0 0 0 1.452 1.074a1.5 1.5 0 0 0 1.345-.948l1.244-3.041H13a1 1 0 1 0 0-2h-2.166a1.51 1.51 0 0 0-1.39.944L8.627 9.02L6.82 3.01a1.48 1.48 0 0 0-1.304-1.065a1.49 1.49 0 0 0-1.471.82L2.387 6.08H1a1 1 0 0 0 0 2h1.701a1.52 1.52 0 0 0 1.333-.823l1.233-2.465Z"
          />
        </svg>
        React
      </h1>
      <p className="text-center mt-2 text-md text-gray-800">
        <strong>esm.sh/hot</strong>{" "}
        gives you the new developer experience for building web applications
        <br />
        in modern browser, quickly.
      </p>
      <div className="flex gap-6 mt-4">
        {externalLinks.map(([text, href]) => (
          <a
            className="flex items-center gap-1 text-gray-400 hover:text-gray-900 transition-colors duration-300"
            href={href}
            target="_blank"
            key={href}
          >
            {text}
            <svg
              className="w-4 h-4"
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 24 24"
            >
              <path
                stroke="currentColor"
                strokeWidth="2.5"
                d="M7 7h10m0 0v10m0-10L7 17"
              />
            </svg>
          </a>
        ))}
      </div>
      <nav className="mt-8">
        <button
          className="inline-flex items-center justify-center gap-2 w-60 h-12 border-1 border-gray-300 rounded-full hover:border-gray-400 transition-colors duration-300"
          onClick={() => setCount(count + 1)}
        >
          Counter: <code>{count}</code>
        </button>
      </nav>
    </div>
  );
}
