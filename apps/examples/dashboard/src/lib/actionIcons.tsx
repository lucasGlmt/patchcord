import AccessTimeIcon from "@mui/icons-material/AccessTime";
import DataObjectIcon from "@mui/icons-material/DataObject";
import ExtensionIcon from "@mui/icons-material/Extension";
import HttpIcon from "@mui/icons-material/Http";
import SmartToyIcon from "@mui/icons-material/SmartToy";
import StorageIcon from "@mui/icons-material/Storage";
import SwapHorizIcon from "@mui/icons-material/SwapHoriz";
import TextFieldsIcon from "@mui/icons-material/TextFields";
import type { ReactElement } from "react";

// Maps a step's `uses` action id (e.g. "postgresql.query@1") to a small
// icon by its plugin-name prefix — purely decorative, so an action from a
// plugin this map doesn't know about just falls back to a generic icon
// rather than needing this list kept exhaustive.
const iconsByPrefix: Record<string, ReactElement> = {
  text: <TextFieldsIcon fontSize="small" />,
  http: <HttpIcon fontSize="small" />,
  postgresql: <StorageIcon fontSize="small" />,
  mysql: <StorageIcon fontSize="small" />,
  openai: <SmartToyIcon fontSize="small" />,
  json: <DataObjectIcon fontSize="small" />,
  encoding: <SwapHorizIcon fontSize="small" />,
  time: <AccessTimeIcon fontSize="small" />,
};

export function actionIcon(uses: string): ReactElement {
  const prefix = uses.split(".")[0];
  return iconsByPrefix[prefix] ?? <ExtensionIcon fontSize="small" />;
}
