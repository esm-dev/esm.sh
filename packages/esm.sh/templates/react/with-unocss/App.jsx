import { useState } from "react";

const externalLinks = [
  ["Docs", "https://esm.sh/docs#hot"],
  ["Templates", "https://esm.sh/new"],
  ["Github", "https://github.com/esm-dev/esm.sh"],
];

export default function App() {
  const [count, setCount] = useState(0);

  return (
    <div className="w-screen h-screen flex flex-col items-center justify-center">
      <p
        className="fixed top-6 op-50 text-xs font-italic select-none cursor-pointer"
        onClick={() => setCount(count + 1)}
      >
        ^^ Made with 💛, React & UnoCSS. ({count})
      </p>

      <h1 className="flex items-center gap-3 text-[40px] mt-6 font-600 leading-none text-center">
        Develop stuff <em>with</em>

        <svg
          className="w-8 h-8 top-0.8 relative"
          width="24"
          height="24"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.4"
          xmlns="http://www.w3.org/2000/svg"
        >
          <defs>
            <clipPath id="--clip-circle">
              <circle cx="12" cy="12" r="10.5" fill="white" stroke="none" />
            </clipPath>
            <ellipse id="--ellipse" cx="12" cy="12" ry="3.75" rx="14.5" />
          </defs>

          <path d="M11.9636 22.5C6.18472 22.5 1.5 17.799 1.5 12C1.5 6.20101 6.18472 1.5 11.9636 1.5">
            <animate
              attributeName="d"
              to="M12.0364 22.5C17.8153 22.5 22.5 17.799 22.5 12C22.5 6.20101 17.8153 1.5 12.0364 1.5"
              dur="3s"
              repeatCount="indefinite"
            />
          </path>
          <path d="M11.9636 22.5C6.18472 22.5 1.5 17.799 1.5 12C1.5 6.20101 6.18472 1.5 11.9636 1.5">
            <animate
              attributeName="d"
              to="M12.0364 22.5C17.8153 22.5 22.5 17.799 22.5 12C22.5 6.20101 17.8153 1.5 12.0364 1.5"
              dur="3s"
              begin="1s"
              repeatCount="indefinite"
            />
          </path>
          <path d="M11.9636 22.5C6.18472 22.5 1.5 17.799 1.5 12C1.5 6.20101 6.18472 1.5 11.9636 1.5">
            <animate
              attributeName="d"
              to="M12.0364 22.5C17.8153 22.5 22.5 17.799 22.5 12C22.5 6.20101 17.8153 1.5 12.0364 1.5"
              dur="3s"
              begin="2s"
              repeatCount="indefinite"
            />
          </path>
          <path d="M12.0364 22.5C17.8153 22.5 22.5 17.799 22.5 12C22.5 6.20101 17.8153 1.5 12.0364 1.5">
            <animate
              attributeName="d"
              from="M11.9636 22.5C9.75448 22.5 7.96362 17.799 7.96362 12C7.96362 6.20101 9.75448 1.5 11.9636 1.5"
              to="M12.0364 22.5C17.8153 22.5 22.5 17.799 22.5 12C22.5 6.20101 17.8153 1.5 12.0364 1.5"
              dur="2s"
            />
          </path>
          <path d="M12.0364 22.5C17.8153 22.5 22.5 17.799 22.5 12C22.5 6.20101 17.8153 1.5 12.0364 1.5">
            <animate
              attributeName="d"
              from="M12.0364 22.5C14.2455 22.5 16.0364 17.799 16.0364 12C16.0364 6.20101 14.2455 1.5 12.0364 1.5"
              to="M12.0364 22.5C17.8153 22.5 22.5 17.799 22.5 12C22.5 6.20101 17.8153 1.5 12.0364 1.5"
              dur="1s"
            />
          </path>

          <use clipPath="url(#--clip-circle)" href="#--ellipse" />

          <circle cx="12" cy="12" r="10.5" />
        </svg>

        browser tech.
      </h1>

      <nav className="fixed bottom-4 flex items-center gap-6 mt-4">
        {externalLinks.map(([text, href]) => (
          <a
            className="flex items-center gap-0.5 py-2 leading-none op-40 hover:op-100 transition-top duration-300 relative top-0 hover:top--0.5"
            target="_blank"
            href={href}
            key={href}
          >
            {text}
            <svg
              className="w-4 h-4 op-80"
              width="24"
              height="24"
              viewBox="0 0 24 24"
              fill="none"
              xmlns="http://www.w3.org/2000/svg"
            >
              <path
                d="M7 7h10m0 0v10m0-10L7 17"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
              />
            </svg>
          </a>
        ))}
        <button
          className="p1.5 rounded-full bg-gray-400/10 hover:bg-gray-400/20 group"
          onClick={() => {
            document.documentElement.classList.toggle("dark");
          }}
        >
          <svg
            className="w-4.5 h-4.5 op-40 group-hover:op-100 transition-opacity duration-300"
            width="24"
            height="24"
            viewBox="0 0 24 24"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
          >
            <path
              d="M14.828 14.828a4 4 0 1 0-5.656-5.656a4 4 0 0 0 5.656 5.656m-8.485 2.829l-1.414 1.414M6.343 6.343L4.929 4.929m12.728 1.414l1.414-1.414m-1.414 12.728l1.414 1.414M4 12H2m10-8V2m8 10h2m-10 8v2"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        </button>
      </nav>
    </div>
  );
}
