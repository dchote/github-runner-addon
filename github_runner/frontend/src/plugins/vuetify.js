import { createVuetify } from 'vuetify'
import { aliases, mdi } from 'vuetify/iconsets/mdi'
import 'vuetify/styles'
import '@mdi/font/css/materialdesignicons.css'
import '@/styles/theme.scss'

// Material Design 3 blue palette (from go-mumble-server)
const brandTheme = {
  light: {
    dark: false,
    colors: {
      primary: '#1976D2',
      'primary-darken-1': '#1565C0',
      secondary: '#424242',
      'secondary-darken-1': '#303030',
      accent: '#2196F3',
      success: '#4CAF50',
      warning: '#FF9800',
      error: '#F44336',
      info: '#2196F3',
      background: '#F5F5F5',
      surface: '#FFFFFF',
      'surface-variant': '#EEEEEE',
      'on-surface-variant': '#424242',
      'on-primary': '#FFFFFF',
      'on-secondary': '#FFFFFF',
      'on-accent': '#FFFFFF',
      'on-error': '#FFFFFF',
      'on-surface': '#212121',
      'on-background': '#212121',
    },
  },
  dark: {
    dark: true,
    colors: {
      primary: '#42A5F5',
      'primary-darken-1': '#1E88E5',
      secondary: '#757575',
      'secondary-darken-1': '#616161',
      accent: '#64B5F6',
      success: '#66BB6A',
      warning: '#FFA726',
      error: '#EF5350',
      info: '#42A5F5',
      background: '#121212',
      surface: '#1E1E1E',
      'surface-variant': '#2C2C2C',
      'on-surface-variant': '#E0E0E0',
      'on-primary': '#0D47A1',
      'on-secondary': '#FFFFFF',
      'on-accent': '#0D47A1',
      'on-error': '#000000',
      'on-surface': '#E0E0E0',
      'on-background': '#E0E0E0',
    },
  },
}

export default createVuetify({
  theme: {
    defaultTheme: 'light',
    themes: brandTheme,
  },
  icons: {
    defaultSet: 'mdi',
    aliases,
    sets: { mdi },
  },
  defaults: {
    VBtn: { rounded: '0' },
    VTextField: {
      variant: 'outlined',
      density: 'comfortable',
      hideDetails: 'auto',
    },
    VDataTable: {
      itemsPerPage: -1,
      hideDefaultFooter: true,
    },
  },
})
