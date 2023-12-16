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
      <h1 className="flex items-center gap-3 text-4xl mt-6 font-600 leading-none text-center">
        Develop stuff with
        <svg
          className="w-7 h-7 top-0.5 relative"
          width="24"
          height="24"
          viewBox="0 0 24 24"
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
        >
          <path
            d="M12 22.5C17.799 22.5 22.5 17.799 22.5 12C22.5 6.20101 17.799 1.5 12 1.5C6.20101 1.5 1.5 6.20101 1.5 12C1.5 17.799 6.20101 22.5 12 22.5Z"
            stroke="currentColor"
            strokeWidth="2.8"
          />
          <path
            d="M1.5 9.32194C4.20562 8.65886 7.91182 8.25 12 8.25C16.0881 8.25 19.7943 8.65886 22.5 9.32194M1.5 14.6781C4.20562 15.3411 7.91182 15.75 12 15.75C16.0881 15.75 19.7943 15.3411 22.5 14.6781"
            stroke="currentColor"
            strokeWidth="2.8"
          />
          <path
            d="M12 1.5C12.2286 1.5 12.5646 1.59957 13.001 2.02144C13.4445 2.45029 13.9125 3.14326 14.3381 4.11595C15.1872 6.05686 15.75 8.84314 15.75 12C15.75 15.1569 15.1872 17.9432 14.3381 19.8841C13.9125 20.8568 13.4445 21.5497 13.001 21.9785C12.5646 22.4004 12.2286 22.5 12 22.5C11.7714 22.5 11.4354 22.4004 10.999 21.9785C10.5555 21.5497 10.0875 20.8568 9.66189 19.8841C8.8128 17.9432 8.25 15.1569 8.25 12C8.25 8.84314 8.8128 6.05686 9.66189 4.11595C10.0875 3.14326 10.5555 2.45029 10.999 2.02144C11.4354 1.59957 11.7714 1.5 12 1.5Z"
            stroke="currentColor"
            strokeWidth="2.8"
          />
        </svg>
        browser tech.
      </h1>
      <nav className="fixed bottom-5 flex gap-6 mt-4">
        {externalLinks.map(([text, href]) => (
          <a
            className="flex items-center gap-1 text-gray-400 hover:text-gray-900 transition-colors duration-300"
            href={href}
            target="_blank"
            key={href}
          >
            {text}
            <svg
              className="w-4 h-4 op-80"
              width="24"
              height="24"
              viewBox="0 0 24 24"
              xmlns="http://www.w3.org/2000/svg"
            >
              <path
                stroke="currentColor"
                strokeWidth="2"
                d="M7 7h10m0 0v10m0-10L7 17"
              />
            </svg>
          </a>
        ))}
      </nav>
    </div>
  );
}
