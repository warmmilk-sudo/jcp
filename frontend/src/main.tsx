import React from 'react'
import {createRoot} from 'react-dom/client'
import './style.css'
import App from './App'
import { ThemeProvider } from './contexts/ThemeContext'
import { CandleColorProvider } from './contexts/CandleColorContext'
import { IndicatorProvider } from './contexts/IndicatorContext'

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
    <React.StrictMode>
        <ThemeProvider>
            <CandleColorProvider>
                <IndicatorProvider>
                    <App/>
                </IndicatorProvider>
            </CandleColorProvider>
        </ThemeProvider>
    </React.StrictMode>
)
