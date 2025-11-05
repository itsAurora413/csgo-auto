import json, re
from pathlib import Path
from datetime import datetime, timezone, timedelta
import pandas as pd, numpy as np
import matplotlib.pyplot as plt
import plotly.graph_objects as go
from plotly.subplots import make_subplots
plt.rcParams['figure.dpi']=120

p=Path('/Users/user/Desktop/kline1.txt')
text = p.read_text(encoding='utf-8', errors='ignore')

# find the "data" array start
m = re.search(r'"data"\s*:\s*\[', text)
if not m:
    # try find first '[' after first '{'
    start = text.find('[')
else:
    start = m.end()-1  # position at '['
# find matching closing bracket for this top-level array
depth = 0
end = None
for i in range(start, len(text)):
    ch = text[i]
    if ch == '[':
        depth += 1
    elif ch == ']':
        depth -= 1
        if depth == 0:
            end = i
            break
if end is None:
    # fallback: take until last occurrence of '}]'
    last = text.rfind('}]')
    if last!=-1:
        end = last+1
    else:
        end = len(text)-1

data_str = text[start:end+1]
# attempt to wrap and load
try:
    obj = json.loads('{"data":' + data_str + '}')
    data = obj['data']
except Exception as e:
    # fallback: attempt to extract lines like {"t":...} using regex
    items = re.findall(r'\{[^{}]*"t"[^{}]*\}', text)
    data = []
    for it in items:
        try:
            data.append(json.loads(it))
        except:
            pass

# build dataframe
rows=[]
for d in data:
    try:
        t=int(d['t'])
        dt=datetime.fromtimestamp(t/1000,tz=timezone.utc)+timedelta(hours=8)
        rows.append({'datetime':dt,'open':float(d['o']),'high':float(d['h']),'low':float(d['l']),'close':float(d['c']),'volume':float(d.get('v',0))})
    except Exception as e:
        continue

df = pd.DataFrame(rows).set_index('datetime').sort_index()
df = df[~df.index.duplicated(keep='first')]
df = df.resample('h').ffill()

# compute indicators (same as before)
def ema(s,span): return s.ewm(span=span,adjust=False).mean()
df['ma5']=df['close'].rolling(5).mean(); df['ma20']=df['close'].rolling(20).mean(); df['ma60']=df['close'].rolling(60).mean()
ema12=ema(df['close'],12); ema26=ema(df['close'],26)
df['dif']=ema12-ema26; df['dea']=df['dif'].ewm(span=9,adjust=False).mean(); df['macd_hist']=2*(df['dif']-df['dea'])
delta=df['close'].diff(); up=delta.clip(lower=0); down=-delta.clip(upper=0)
df['rsi14']=100 - 100/(1 + (up.rolling(14).mean()/(down.rolling(14).mean()+1e-9)))
df['mb']=df['close'].rolling(20).mean(); df['std20']=df['close'].rolling(20).std(); df['upper_bb']=df['mb']+2*df['std20']; df['lower_bb']=df['mb']-2*df['std20']; df['bb_width']=df['upper_bb']-df['lower_bb']

# consolidation detection
thr = df['bb_width'].quantile(0.25)
df['narrow']=df['bb_width']<thr
df['grp']=(df['narrow']!=df['narrow'].shift(1)).cumsum()
groups = df.groupby('grp')['narrow'].agg(['first','sum','size'])
narrow_periods = groups[(groups['first']==True)&(groups['sum']>=24)]
consolation_zone=None
if not narrow_periods.empty:
    last_gid = narrow_periods.index.max()
    idx = df[df['grp']==last_gid].index
    consolation_zone=(idx[0], idx[-1], float(df.loc[idx,'close'].min()), float(df.loc[idx,'close'].max()))

# Simple forecast: linear trend + last-week mean as baseline
series = df['close'].dropna()
h1=24*7; h2=24*30; h3=24*90; hmax=h3
pred_index = pd.date_range(start=series.index[-1]+pd.Timedelta(hours=1), periods=hmax, freq='h', tz=series.index.tz)
# linear trend on last 168 hours
y = series.values; x = np.arange(len(y))
if len(y) >= 200:
    coef = np.polyfit(x[-168:], y[-168:], 1)
else:
    coef = np.polyfit(x, y, 1)
trend = np.poly1d(coef)
future_x = np.arange(len(y), len(y)+hmax)
ar_pred = pd.Series(trend(future_x), index=pred_index)
resid_std = np.std(y - trend(x))
# ml fallback: moving average + small noise
ma_week = series.rolling(24*7, min_periods=1).mean().iloc[-1]
ml_pred = pd.Series(ma_week + 0.0*(np.arange(hmax)), index=pred_index)
ml_std = np.std(series - series.rolling(24*7,min_periods=1).mean())

# Buy/sell zones
recent = df.iloc[-1]
buy_zones=[]; sell_zones=[]
if consolation_zone:
    buy_zones.append({'type':'consolidation_bottom','start':consolation_zone[0],'end':consolation_zone[1],'price_low':consolation_zone[2],'price_high':consolation_zone[3]})
dyn_buy = max(df['lower_bb'].iloc[-1], df['ma60'].iloc[-1]*0.98)
if recent['rsi14'] < 50:
    buy_zones.append({'type':'dynamic','price':float(dyn_buy),'rsi':float(recent['rsi14'])})
dyn_sell = min(df['upper_bb'].iloc[-1], df['ma20'].iloc[-1]*1.02)
if recent['rsi14'] > 55:
    sell_zones.append({'type':'dynamic','price':float(dyn_sell),'rsi':float(recent['rsi14'])})
if consolation_zone:
    sell_zones.append({'type':'consolidation_top','price_high':float(consolation_zone[3])})

# Plotting main chart using Plotly with larger span
fig_tech = make_subplots(
    rows=3, cols=1,
    shared_xaxes=True,
    row_heights=[0.5, 0.25, 0.25],
    subplot_titles=('CSGO饰品指数 - 小时K线与技术指标 (北京时间 GMT+8)', 'MACD', 'RSI'),
    vertical_spacing=0.08
)

times = df.index

# Candlestick chart - more efficient approach
# High-Low lines
fig_tech.add_trace(
    go.Scatter(x=times, y=df['high'], mode='lines', line=dict(color='rgba(0,0,0,0)', width=0),
              showlegend=False, hoverinfo='skip'),
    row=1, col=1
)
fig_tech.add_trace(
    go.Scatter(x=times, y=df['low'], mode='lines', line=dict(color='rgba(0,0,0,0)', width=0),
              fill='tonexty', fillcolor='rgba(0,0,0,0.3)',
              showlegend=False, hoverinfo='skip'),
    row=1, col=1
)

# Open-Close bars
colors = ['red' if df['close'].iloc[i] >= df['open'].iloc[i] else 'green' for i in range(len(df))]
fig_tech.add_trace(
    go.Bar(x=times, y=df['close']-df['open'], base=df['open'], 
           marker=dict(color=colors), width=0.6*3600*1000,
           showlegend=False, hovertemplate='<b>%{x}</b><br>Open: %{base}<br>Close: %{y}<extra></extra>'),
    row=1, col=1
)

# Moving averages
fig_tech.add_trace(go.Scatter(x=times, y=df['ma5'], name='MA5', line=dict(width=1)), row=1, col=1)
fig_tech.add_trace(go.Scatter(x=times, y=df['ma20'], name='MA20', line=dict(width=1)), row=1, col=1)
fig_tech.add_trace(go.Scatter(x=times, y=df['ma60'], name='MA60', line=dict(width=1)), row=1, col=1)

# Bollinger Bands
fig_tech.add_trace(go.Scatter(x=times, y=df['upper_bb'], name='Upper BB', 
                              line=dict(width=0.8, dash='dash')), row=1, col=1)
fig_tech.add_trace(go.Scatter(x=times, y=df['lower_bb'], name='Lower BB', 
                              line=dict(width=0.8, dash='dash')), row=1, col=1)

# Consolidation zone
if consolation_zone:
    fig_tech.add_vrect(x0=consolation_zone[0], x1=consolation_zone[1], 
                       fillcolor='gray', opacity=0.15, layer='below', row=1, col=1)

# MACD
fig_tech.add_trace(go.Scatter(x=times, y=df['dif'], name='DIF', line=dict(width=1)), row=2, col=1)
fig_tech.add_trace(go.Scatter(x=times, y=df['dea'], name='DEA', line=dict(width=1)), row=2, col=1)
fig_tech.add_trace(go.Bar(x=times, y=df['macd_hist'], name='MACD Hist', opacity=0.6), row=2, col=1)
fig_tech.add_hline(y=0, line_dash='solid', line_width=0.5, row=2, col=1)

# RSI
fig_tech.add_trace(go.Scatter(x=times, y=df['rsi14'], name='RSI14', line=dict(width=1)), row=3, col=1)
fig_tech.add_hline(y=70, line_dash='dash', line_width=0.5, line_color='red', row=3, col=1)
fig_tech.add_hline(y=30, line_dash='dash', line_width=0.5, line_color='green', row=3, col=1)

fig_tech.update_yaxes(title_text='Index Value', row=1, col=1)
fig_tech.update_yaxes(title_text='MACD', row=2, col=1)
fig_tech.update_yaxes(title_text='RSI', row=3, col=1)
fig_tech.update_xaxes(title_text='Datetime (GMT+8)', row=3, col=1)

fig_tech.update_layout(
    title='CSGO饰品指数 - 小时K线与技术指标 (北京时间 GMT+8)',
    hovermode='x unified',
    height=800,
    width=1600,
    template='plotly_white'
)

out1_html='/Users/user/Desktop/csgo_technical.html'
fig_tech.write_html(out1_html)

# Prediction plot using Plotly
fig_pred = go.Figure()
hist_display = series[-24*14:]
fig_pred.add_trace(go.Scatter(x=hist_display.index, y=hist_display.values, 
                             name='Historical (last 14 days)', mode='lines', line=dict(width=2)))
fig_pred.add_trace(go.Scatter(x=ar_pred.index[:h1], y=ar_pred.values[:h1], 
                             name='Trend (linear) forecast (1w)', mode='lines', 
                             line=dict(width=2, dash='dash')))
fig_pred.add_trace(go.Scatter(x=ml_pred.index[:h1], y=ml_pred.values[:h1], 
                             name='MA-week baseline (1w)', mode='lines', 
                             line=dict(width=2, dash='dot')))

# Confidence intervals
fig_pred.add_trace(go.Scatter(
    x=list(ar_pred.index[:h1]) + list(ar_pred.index[:h1][::-1]),
    y=list(ar_pred.values[:h1]+1.96*resid_std) + list((ar_pred.values[:h1]-1.96*resid_std)[::-1]),
    fill='toself', fillcolor='rgba(128, 128, 128, 0.2)', line=dict(color='rgba(255,255,255,0)'),
    name='Trend 95% CI', hoverinfo='skip'
))

fig_pred.add_trace(go.Scatter(
    x=list(ml_pred.index[:h1]) + list(ml_pred.index[:h1][::-1]),
    y=list(ml_pred.values[:h1]+1.96*ml_std) + list((ml_pred.values[:h1]-1.96*ml_std)[::-1]),
    fill='toself', fillcolor='rgba(255, 165, 0, 0.12)', line=dict(color='rgba(255,255,255,0)'),
    name='MA CI', hoverinfo='skip'
))

fig_pred.update_layout(
    title='预测对比（未来1周示意）',
    xaxis_title='Datetime (GMT+8)',
    yaxis_title='Index Value',
    hovermode='x unified',
    height=500,
    width=1600,
    template='plotly_white'
)

out2_html='/Users/user/Desktop/csgo_forecast.html'
fig_pred.write_html(out2_html)

# Save predictions csv
pred_df = pd.DataFrame({'ar_pred':ar_pred, 'ml_pred':ml_pred})
pred_df.to_csv('/Users/user/Desktop/predictions_3months.csv')

result = {
    'chart_technical': out1_html,
    'chart_forecast': out2_html,
    'pred_csv': '/Users/user/Desktop/predictions_3months.csv',
    'consolidation_zone': consolation_zone,
    'buy_zones': buy_zones,
    'sell_zones': sell_zones,
    'last_close': float(series.iloc[-1])
}
result