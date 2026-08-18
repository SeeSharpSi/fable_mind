# Fable Mind 🤖✍️

**Fable Mind is an interactive, text-based adventure game powered by an OpenAI-compatible API.** Players craft a unique story by responding to AI-generated scenarios, with their choices directly shaping the narrative, the items they discover, and the world they explore.

---

<p align="center">
  <img src="https://raw.githubusercontent.com/SeeSharpSi/silasblog/refs/heads/main/static/mark_twain_scifi.png" alt="A robot hand writing a fantasy story in a book" width=600>
</p>

---

## ✨ Features

*   **Systemic, Winnable Gameplay:** The AI acts as a Game Master, managing a persistent world state and presenting solvable challenges. Player agency is paramount.
*   **Dynamic AI Storytelling:** Powered by your configured language model, every adventure is unique and unpredictable. The story adapts to your choices in real-time.
*   **Author-Styled Narratives:** Begin your adventure in the literary style of a famous author like J.R.R. Tolkien, H.P. Lovecraft, or Edgar Allan Poe for a unique narrative flavor.
*   **Selectable Difficulty:** Choose your preferred playstyle:
    *   **Exploratory:** A forgiving mode focused on story and discovery.
    *   **Challenging:** A balanced experience with real risks and rewards.
    *   **Punishing:** A hardcore mode where poor choices can have severe and deadly consequences.
*   **Genre-Themed UI:** The color scheme of the app changes to a unique dark theme based on your chosen genre (Fantasy, Sci-Fi, or Historical Fiction).
*   **Interactive Inventory & World:** The AI tracks items, which have properties and can be used to solve puzzles by interacting with objects in the environment.
*   **Subtle State Display:** Keep track of your health and item properties through an immersive, minimalist UI without breaking the narrative flow.
*   **Download Your Story:** Once your adventure concludes, you can download the entire story as a beautifully formatted PDF to save or share.
*   **Modern, Fast Frontend:** The UI is built with Go, HTMX, and Templ, delivering a seamless, server-rendered experience without heavy client-side JavaScript.

## 🛠️ Tech Stack

*   **Backend:** Go
*   **Frontend:** HTMX & [a-h/templ](https://github.com/a-h/templ)
*   **AI Model:** Any OpenAI-compatible Chat Completions endpoint
*   **PDF Generation:** [gofpdf](https://github.com/jung-kurt/gofpdf)

## 🚀 Getting Started

Follow these steps to get Story AI running on your local machine.

### 1. Prerequisites

*   Go 1.24.5 or later.
*   Access to an OpenAI-compatible Chat Completions endpoint.

### 2. Clone the Repository

```bash
git clone https://github.com/your-username/story_ai
cd story_ai
```

### 3. Set Up Your Environment

Create a `.env` file in the root of the project directory:

```dotenv
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_API_KEY=your-api-key
OPENAI_MODEL=gpt-4o-mini
```

`OPENAI_BASE_URL` may be either a base API URL or the full `/chat/completions` URL. `OPENAI_API_KEY` may be empty for local endpoints that do not require authentication. `OPENAI_MODEL` is required.

`OPENAI_TEMPERATURE` defaults to `0.65`. `OPENAI_START_MAX_TOKENS` and `OPENAI_TURN_MAX_TOKENS` default to `3000` and `1600`; raise them if a model truncates valid JSON.

`OPENAI_RESPONSE_FORMAT` defaults to `json_object`. Set it to `none` only for compatible endpoints that reject `response_format`; server-side parsing remains strict.

OpenCode Go example:

```dotenv
OPENAI_BASE_URL=https://opencode.ai/zen/go/v1
OPENAI_API_KEY=your-opencode-go-api-key
OPENAI_MODEL=kimi-k3
```

Use an OpenCode Go model exposed through `/chat/completions`; models available only through Anthropic-compatible `/messages` are not supported by this backend.

### 4. Run the Application

First, ensure the templ files are generated:
```bash
templ generate
```

Then, run the application:
```bash
go run .
```

The application will be available at `http://localhost:9779`.

### Run with Podman

Build the image and pass API configuration at runtime:

```bash
podman build -t story-ai .
podman run --rm --name story-ai -p 9779:9779 --env-file .env story-ai
```

The image contains the bundled `data.db`; `.env` is excluded from the image.

## 🎮 How to Play

1.  Open your web browser to `http://localhost:9779`.
2.  **Choose Your Adventure:** Select a genre (Fantasy, Sci-Fi, or Historical Fiction).
3.  **Choose Your Difficulty:** Select a difficulty level from the dropdown (Exploratory, Challenging, or Punishing).
4.  Click a genre button to begin!
5.  Read the AI-generated scenario and type your response (15 words or less) into the input box.
6.  Click "Send" and watch the story unfold based on your choices.
7.  If your story reaches a conclusion, you can restart or download your adventure as a PDF. Good luck!
