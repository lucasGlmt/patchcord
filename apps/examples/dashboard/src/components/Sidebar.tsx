import AppsIcon from "@mui/icons-material/Apps";
import CableIcon from "@mui/icons-material/Cable";
import HistoryIcon from "@mui/icons-material/History";
import ListAltIcon from "@mui/icons-material/ListAlt";
import Box from "@mui/material/Box";
import Drawer from "@mui/material/Drawer";
import List from "@mui/material/List";
import ListItemButton from "@mui/material/ListItemButton";
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText from "@mui/material/ListItemText";
import Typography from "@mui/material/Typography";
import { useLocation, useNavigate } from "react-router-dom";

export const sidebarWidth = 220;

const navItems = [
  { label: "Workflows", path: "/workflows", icon: <ListAltIcon fontSize="small" /> },
  { label: "Runs", path: "/runs", icon: <HistoryIcon fontSize="small" /> },
  { label: "Connectors", path: "/connectors", icon: <CableIcon fontSize="small" /> },
  { label: "Apps", path: "/apps", icon: <AppsIcon fontSize="small" /> },
];

export default function Sidebar() {
  const location = useLocation();
  const navigate = useNavigate();

  return (
    <Drawer
      variant="permanent"
      sx={{
        width: sidebarWidth,
        flexShrink: 0,
        "& .MuiDrawer-paper": { width: sidebarWidth, borderRight: "1px solid", borderColor: "divider" },
      }}
    >
      <Box sx={{ px: 2, py: 2.5 }}>
        <Typography variant="subtitle1" fontWeight={800} letterSpacing={-0.3}>
          Patchcord
        </Typography>
        <Typography variant="caption" color="text.secondary">
          Operator console
        </Typography>
      </Box>
      <List sx={{ px: 1 }}>
        {navItems.map((item) => {
          const selected = location.pathname === item.path || location.pathname.startsWith(`${item.path}/`);
          return (
            <ListItemButton
              key={item.path}
              selected={selected}
              onClick={() => navigate(item.path)}
              sx={{ borderRadius: 1.5, mb: 0.5 }}
            >
              <ListItemIcon sx={{ minWidth: 34 }}>{item.icon}</ListItemIcon>
              <ListItemText primaryTypographyProps={{ variant: "body2", fontWeight: selected ? 700 : 500 }}>
                {item.label}
              </ListItemText>
            </ListItemButton>
          );
        })}
      </List>
    </Drawer>
  );
}
