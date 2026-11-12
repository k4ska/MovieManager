# MovieManager

A terminal browser for a DVD collection kept in a Google Sheet. Browse, search,
filter and sort your movies, see stats, and add or edit movies with details
filled in automatically from TMDB.

## Install

Needs Go 1.27 or newer.

```sh
git clone https://github.com/k4ska/MovieManager.git
cd MovieManager
go build
./moviemanager
```

## Setup

### 1. The Google Sheet

Make a Google Sheet whose first tab has these column headers, in any order:

```
id, title, year, genre, studio, language, duration
```

Share it as **Anyone with the link → Viewer** (that's all browsing needs), then
tell the app where it is by pasting the sheet's URL (or just its ID) into a file:

```sh
mkdir -p ~/.config/moviemanager
echo "https://docs.google.com/spreadsheets/d/<your-sheet-id>/edit" > ~/.config/moviemanager/sheet
```

Or set it as an environment variable instead: `export MOVIEMANAGER_SHEET=<url-or-id>`.

### 2. Google Sheets API (for adding, editing and deleting)

Changes are written through a Google service account:

1. Go to the [Google Cloud Console](https://console.cloud.google.com/) and create a project.
2. Open **APIs & Services → Library**, search for **Google Sheets API** and enable it.
3. Open **IAM & Admin → Service Accounts** and create a service account (no roles needed).
4. On the service account, go to **Keys → Add key → Create new key → JSON**. A `.json` file downloads.
5. In your Google Sheet, click **Share** and add the service account's email
   (`...@...iam.gserviceaccount.com`) as an **Editor**.
6. Save the key file as:

```sh
mv ~/Downloads/<your-key>.json ~/.config/moviemanager/service-account.json
```

### 3. TMDB API (for auto-filling movie details)

1. Create a free account at [themoviedb.org](https://www.themoviedb.org/signup).
2. Go to **Settings → API** ([direct link](https://www.themoviedb.org/settings/api)) and request an API key.
3. Copy the **API Read Access Token** (the long one, not the short "API Key").
4. Save it as:

```sh
echo "<your-token>" > ~/.config/moviemanager/tmdb-token
```

Or set it as an environment variable instead: `export TMDB_TOKEN=<your-token>`.

## Where things go

| File                                         | What                         |
| -------------------------------------------- | ---------------------------- |
| `~/.config/moviemanager/sheet`               | Google Sheet URL or ID       |
| `~/.config/moviemanager/service-account.json` | Google service account key   |
| `~/.config/moviemanager/tmdb-token`          | TMDB API Read Access Token   |

The app uses `$XDG_CONFIG_HOME/moviemanager` if that is set. Never commit
`service-account.json` or `tmdb-token`: they are secret.
