/**
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 * http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
export type Nullable<T> = T | null;
export interface NeighborData<D> {
	prev: D;
	current: D;
	next: D;
}
export interface KLineData {
	timestamp: number;
	open: number;
	high: number;
	low: number;
	close: number;
	volume?: number;
	turnover?: number;
	[key: string]: unknown;
}
export interface Margin {
	marginLeft: number;
	marginTop: number;
	marginRight: number;
	marginBottom: number;
}
export interface Padding {
	paddingLeft: number;
	paddingTop: number;
	paddingRight: number;
	paddingBottom: number;
}
export interface Offset {
	offsetLeft: number;
	offsetTop: number;
	offsetRight: number;
	offsetBottom: number;
}
/**
 * line type
 */
export declare enum LineType {
	Dashed = "dashed",
	Solid = "solid"
}
export interface LineStyle {
	style: LineType;
	size: number;
	color: string;
	dashedValue: number[];
}
export interface SmoothLineStyle extends LineStyle {
	smooth: boolean | number;
}
export interface StateLineStyle extends LineStyle {
	show: boolean;
}
declare enum PathType {
	Stroke = "stroke",
	Fill = "fill"
}
export interface PathStyle {
	style: PathType;
	color: string;
	lineWidth: number;
}
export declare enum PolygonType {
	Stroke = "stroke",
	Fill = "fill",
	StrokeFill = "stroke_fill"
}
export interface PolygonStyle {
	style: PolygonType;
	color: string | CanvasGradient;
	borderColor: string;
	borderSize: number;
	borderStyle: LineType;
	borderDashedValue: number[];
}
export interface RectStyle extends PolygonStyle {
	borderRadius: number | number[];
}
export interface TextStyle extends Padding {
	style: PolygonType;
	color: string;
	size: number;
	family: string;
	weight: number | string;
	borderStyle: LineType;
	borderDashedValue: number[];
	borderSize: number;
	borderColor: string;
	borderRadius: number | number[];
	backgroundColor: string | CanvasGradient;
}
export interface StateTextStyle extends TextStyle {
	show: boolean;
}
export type LastValueMarkTextStyle = Omit<StateTextStyle, "backgroundColor">;
export declare enum TooltipShowRule {
	Always = "always",
	FollowCross = "follow_cross",
	None = "none"
}
export declare enum TooltipShowType {
	Standard = "standard",
	Rect = "rect"
}
export interface ChangeColor {
	upColor: string;
	downColor: string;
	noChangeColor: string;
}
export interface GradientColor {
	offset: number;
	color: string;
}
export interface GridStyle {
	show: boolean;
	horizontal: StateLineStyle;
	vertical: StateLineStyle;
}
export type TooltipTextStyle = Pick<TextStyle, "color" | "size" | "family" | "weight"> & Margin;
export interface TooltipLegendChild {
	text: string;
	color: string;
}
export interface TooltipLegend {
	title: string | TooltipLegendChild;
	value: string | TooltipLegendChild;
}
export declare enum TooltipFeatureType {
	Path = "path",
	IconFont = "icon_font"
}
export declare enum TooltipFeaturePosition {
	Left = "left",
	Middle = "middle",
	Right = "right"
}
export interface TooltipFeaturePathStyle extends Omit<PathStyle, "color"> {
	path: string;
}
export interface TooltipFeatureIconFontStyle {
	family: string;
	content: string;
}
export interface TooltipFeatureStyle extends Padding, Margin {
	id: string;
	position: TooltipFeaturePosition;
	backgroundColor: string;
	activeBackgroundColor: string;
	size: number;
	color: string;
	activeColor: string;
	borderRadius: number | number[];
	type: TooltipFeatureType;
	path: TooltipFeaturePathStyle;
	iconFont: TooltipFeatureIconFontStyle;
}
export interface TooltipStyle {
	showRule: TooltipShowRule;
	showType: TooltipShowType;
	defaultValue: string;
	text: TooltipTextStyle;
	features: TooltipFeatureStyle[];
}
export interface CandleAreaPointStyle {
	show: boolean;
	color: string;
	radius: number;
	rippleColor: string;
	rippleRadius: number;
	animation: boolean;
	animationDuration: number;
}
export interface CandleAreaStyle {
	lineSize: number;
	lineColor: string;
	value: string;
	smooth: boolean;
	backgroundColor: string | GradientColor[];
	point: CandleAreaPointStyle;
}
export interface CandleHighLowPriceMarkStyle {
	show: boolean;
	color: string;
	textOffset: number;
	textSize: number;
	textFamily: string;
	textWeight: string;
}
export type CandleLastPriceMarkLineStyle = Omit<StateLineStyle, "color">;
export interface CandleLastPriceMarkStyle extends ChangeColor {
	show: boolean;
	compareRule: CandleColorCompareRule;
	line: CandleLastPriceMarkLineStyle;
	text: LastValueMarkTextStyle;
}
export interface CandlePriceMarkStyle {
	show: boolean;
	high: CandleHighLowPriceMarkStyle;
	low: CandleHighLowPriceMarkStyle;
	last: CandleLastPriceMarkStyle;
}
export declare enum CandleTooltipRectPosition {
	Fixed = "fixed",
	Pointer = "pointer"
}
export interface CandleTooltipRectStyle extends Omit<RectStyle, "style" | "borderDashedValue" | "borderStyle">, Padding, Offset {
	position: CandleTooltipRectPosition;
}
export type CandleTooltipCustomCallback = (data: NeighborData<Nullable<KLineData>>, styles: CandleStyle) => TooltipLegend[];
export interface CandleTooltipStyle extends TooltipStyle, Offset {
	custom: CandleTooltipCustomCallback | TooltipLegend[];
	rect: CandleTooltipRectStyle;
}
export declare enum CandleType {
	CandleSolid = "candle_solid",
	CandleStroke = "candle_stroke",
	CandleUpStroke = "candle_up_stroke",
	CandleDownStroke = "candle_down_stroke",
	Ohlc = "ohlc",
	Area = "area"
}
declare enum CandleColorCompareRule {
	CurrentOpen = "current_open",
	PreviousClose = "previous_close"
}
export interface CandleBarColor extends ChangeColor {
	compareRule: CandleColorCompareRule;
	upBorderColor: string;
	downBorderColor: string;
	noChangeBorderColor: string;
	upWickColor: string;
	downWickColor: string;
	noChangeWickColor: string;
}
export interface CandleStyle {
	type: CandleType;
	bar: CandleBarColor;
	area: CandleAreaStyle;
	priceMark: CandlePriceMarkStyle;
	tooltip: CandleTooltipStyle;
}
export type IndicatorPolygonStyle = Omit<PolygonStyle, "color" | "borderColor"> & ChangeColor;
export interface IndicatorLastValueMarkStyle {
	show: boolean;
	text: LastValueMarkTextStyle;
}
export interface IndicatorTooltipStyle extends TooltipStyle, Offset {
	showName: boolean;
	showParams: boolean;
}
export interface IndicatorStyle {
	ohlc: Pick<CandleBarColor, "compareRule" | "upColor" | "downColor" | "noChangeColor">;
	bars: IndicatorPolygonStyle[];
	lines: SmoothLineStyle[];
	circles: IndicatorPolygonStyle[];
	lastValueMark: IndicatorLastValueMarkStyle;
	tooltip: IndicatorTooltipStyle;
	[key: string]: unknown;
}
export type AxisLineStyle = Omit<StateLineStyle, "style" | "dashedValue">;
export interface AxisTickLineStyle extends AxisLineStyle {
	length: number;
}
export interface AxisTickTextStyle extends Pick<StateTextStyle, "show" | "color" | "weight" | "family" | "size"> {
	marginStart: number;
	marginEnd: number;
}
export interface AxisStyle {
	show: boolean;
	size: number | "auto";
	axisLine: AxisLineStyle;
	tickLine: AxisTickLineStyle;
	tickText: AxisTickTextStyle;
}
export interface CrosshairDirectionStyle {
	show: boolean;
	line: StateLineStyle;
	text: StateTextStyle;
}
export interface CrosshairStyle {
	show: boolean;
	horizontal: CrosshairDirectionStyle;
	vertical: CrosshairDirectionStyle;
}
export interface OverlayPointStyle {
	color: string;
	borderColor: string;
	borderSize: number;
	radius: number;
	activeColor: string;
	activeBorderColor: string;
	activeBorderSize: number;
	activeRadius: number;
}
export interface OverlayStyle {
	point: OverlayPointStyle;
	line: SmoothLineStyle;
	rect: RectStyle;
	polygon: PolygonStyle;
	circle: PolygonStyle;
	arc: LineStyle;
	text: TextStyle;
	[key: string]: unknown;
}
export interface SeparatorStyle {
	size: number;
	color: string;
	fill: boolean;
	activeBackgroundColor: string;
}
export interface Styles {
	grid: GridStyle;
	candle: CandleStyle;
	indicator: IndicatorStyle;
	xAxis: AxisStyle;
	yAxis: AxisStyle;
	separator: SeparatorStyle;
	crosshair: CrosshairStyle;
	overlay: OverlayStyle;
}
declare function merge(target: any, source: any): void;
declare function clone<T>(target: T): T;
declare function isArray<T = unknown>(value: unknown): value is T[];
declare function isFunction<T = (...args: unknown[]) => unknown>(value: unknown): value is T;
declare function isObject(value: unknown): value is object;
declare function isNumber(value: unknown): value is number;
declare function isValid<T>(value: T | null | undefined): value is T;
declare function isBoolean(value: unknown): value is boolean;
declare function isString(value: unknown): value is string;
declare function formatValue(data: unknown, key: string, defaultValue?: unknown): unknown;
declare function formatTimestampToString(dateTimeFormat: Intl.DateTimeFormat, timestamp: number, format: string): string;
declare function formatPrecision(value: string | number, precision?: number): string;
declare function formatBigNumber(value: string | number): string;
declare function formatThousands(value: string | number, sign: string): string;
declare function formatFoldDecimal(value: string | number, threshold: number): string;
declare function calcTextWidth(text: string, size?: number, weight?: string | number, family?: string): number;
/**
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 * http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
export type ActionCallback = (data?: unknown) => void;
export declare enum ActionType {
	OnZoom = "onZoom",
	OnScroll = "onScroll",
	OnVisibleRangeChange = "onVisibleRangeChange",
	OnCandleTooltipFeatureClick = "onCandleTooltipFeatureClick",
	OnCrosshairChange = "onCrosshairChange",
	OnCandleBarClick = "onCandleBarClick",
	OnPaneDrag = "onPaneDrag"
}
/**
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 * http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
export type DeepPartial<T> = {
	[P in keyof T]?: T[P] extends Array<infer U> ? Array<DeepPartial<U>> : T[P] extends ReadonlyArray<infer X> ? ReadonlyArray<DeepPartial<X>> : T[P] extends object ? DeepPartial<T[P]> : T[P];
};
/**
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 * http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
export type PickRequired<T, K extends keyof T> = Omit<T, K> & Required<Pick<T, K>>;
export type ExcludePickPartial<T, K extends keyof T> = PickRequired<Partial<T>, K>;
/**
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 * http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
export interface Bounding {
	width: number;
	height: number;
	left: number;
	right: number;
	top: number;
	bottom: number;
}
/**
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 * http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
export interface BarSpace {
	bar: number;
	halfBar: number;
	gapBar: number;
	halfGapBar: number;
}
/**
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 * http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
export interface Coordinate {
	x: number;
	y: number;
}
export interface Crosshair extends Partial<Coordinate> {
	paneId?: string;
	realX?: number;
	timestamp?: number;
	kLineData?: KLineData;
	dataIndex?: number;
	realDataIndex?: number;
}
/**
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 * http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
export interface VisibleRange {
	readonly from: number;
	readonly to: number;
	readonly realFrom: number;
	readonly realTo: number;
}
export interface MouseTouchEvent extends Coordinate {
	pageX: number;
	pageY: number;
	isTouch?: boolean;
	preventDefault?: () => void;
}
/**
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 * http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
export interface Point {
	dataIndex: number;
	timestamp: number;
	value: number;
}
declare enum LoadDataType {
	Init = "init",
	Forward = "forward",
	Backward = "backward",
	Update = "update"
}
export interface LoadDataParams {
	type: LoadDataType;
	data: Nullable<KLineData>;
	callback: (dataList: KLineData[], more?: boolean) => void;
}
export interface LoadDataMore {
	[LoadDataType.Backward]: boolean;
	[LoadDataType.Forward]: boolean;
}
export type LoadDataCallback = (params: LoadDataParams) => void;
/**
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 * http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
export interface Precision {
	price: number;
	volume: number;
}
export declare const enum PaneState {
	Normal = "normal",
	Maximize = "maximize",
	Minimize = "minimize"
}
export interface PaneOptions {
	id?: string;
	height?: number;
	minHeight?: number;
	dragEnabled?: boolean;
	order?: number;
	state?: PaneState;
	axis?: Partial<AxisCreate>;
}
export declare enum FormatDateType {
	Tooltip = 0,
	Crosshair = 1,
	XAxis = 2
}
export type FormatDate = (timestamp: number, format: string, type: FormatDateType) => string;
export type FormatBigNumber = (value: string | number) => string;
export interface CustomApi {
	formatDate: FormatDate;
	formatBigNumber: FormatBigNumber;
}
export interface Locales {
	time: string;
	open: string;
	high: string;
	low: string;
	close: string;
	volume: string;
	change: string;
	turnover: string;
	[key: string]: string;
}
export declare const enum LayoutChildType {
	Candle = "candle",
	Indicator = "indicator",
	XAxis = "xAxis"
}
export interface LayoutChild {
	type: LayoutChildType;
	content?: Array<string | IndicatorCreate>;
	options?: PaneOptions;
}
export interface DecimalFold {
	threshold: number;
	format: (value: string | number) => string;
}
export interface ThousandsSeparator {
	sign: string;
	format: (value: string | number) => string;
}
export interface Options {
	locale?: string;
	timezone?: string;
	styles?: string | DeepPartial<Styles>;
	customApi?: Partial<CustomApi>;
	thousandsSeparator?: Partial<ThousandsSeparator>;
	decimalFold?: Partial<DecimalFold>;
	layout?: LayoutChild[];
}
export type YAxisTemplate = AxisTemplate;
export interface YAxis extends Axis, Required<YAxisTemplate> {
	isFromZero: () => boolean;
	isInCandle: () => boolean;
	convertToNicePixel: (value: number) => number;
}
export declare enum OverlayMode {
	Normal = "normal",
	WeakMagnet = "weak_magnet",
	StrongMagnet = "strong_magnet"
}
export interface OverlayPerformEventParams {
	currentStep: number;
	mode: OverlayMode;
	points: Array<Partial<Point>>;
	performPointIndex: number;
	performPoint: Partial<Point>;
}
export interface OverlayEventCollection<E> {
	onDrawStart: Nullable<OverlayEventCallback<E>>;
	onDrawing: Nullable<OverlayEventCallback<E>>;
	onDrawEnd: Nullable<OverlayEventCallback<E>>;
	onRemoved: Nullable<OverlayEventCallback<E>>;
	onClick: Nullable<OverlayEventCallback<E>>;
	onDoubleClick: Nullable<OverlayEventCallback<E>>;
	onRightClick: Nullable<OverlayEventCallback<E>>;
	onPressedMoveStart: Nullable<OverlayEventCallback<E>>;
	onPressedMoving: Nullable<OverlayEventCallback<E>>;
	onPressedMoveEnd: Nullable<OverlayEventCallback<E>>;
	onMouseEnter: Nullable<OverlayEventCallback<E>>;
	onMouseLeave: Nullable<OverlayEventCallback<E>>;
	onSelected: Nullable<OverlayEventCallback<E>>;
	onDeselected: Nullable<OverlayEventCallback<E>>;
}
export interface OverlayFigure {
	key?: string;
	type: string;
	attrs: unknown;
	styles?: unknown;
	ignoreEvent?: boolean | Array<keyof Omit<OverlayEventCollection<unknown>, "onDrawStart" | "onDrawing" | "onDrawEnd" | "onRemoved">>;
}
export interface OverlayCreateFiguresCallbackParams<E> {
	chart: Chart;
	overlay: Overlay<E>;
	coordinates: Coordinate[];
	bounding: Bounding;
	xAxis: Nullable<XAxis>;
	yAxis: Nullable<YAxis>;
}
export interface OverlayEvent<E> extends Partial<MouseTouchEvent> {
	figure?: OverlayFigure;
	overlay: Overlay<E>;
	chart: Chart;
}
export type OverlayEventCallback<E> = (event: OverlayEvent<E>) => boolean;
export type OverlayCreateFiguresCallback<E> = (params: OverlayCreateFiguresCallbackParams<E>) => OverlayFigure | OverlayFigure[];
export interface Overlay<E = unknown> extends OverlayEventCollection<E> {
	/**
	 * Unique identification
	 */
	id: string;
	/**
	 * Group id
	 */
	groupId: string;
	/**
	 * Pane id
	 */
	paneId: string;
	/**
	 * Name
	 */
	name: string;
	/**
	 * Total number of steps required to complete mouse operation
	 */
	totalStep: number;
	/**
	 * Current step
	 */
	currentStep: number;
	/**
	 * Whether it is locked. When it is true, it will not respond to events
	 */
	lock: boolean;
	/**
	 * Whether the overlay is visible
	 */
	visible: boolean;
	/**
	 * Draw level
	 */
	zLevel: number;
	/**
	 * Whether the default figure corresponding to the point is required
	 */
	needDefaultPointFigure: boolean;
	/**
	 * Whether the default figure on the Y axis is required
	 */
	needDefaultXAxisFigure: boolean;
	/**
	 * Whether the default figure on the X axis is required
	 */
	needDefaultYAxisFigure: boolean;
	/**
	 * Mode
	 */
	mode: OverlayMode;
	/**
	 * When mode is weak_magnet is the response distance
	 */
	modeSensitivity: number;
	/**
	 * Time and value information
	 */
	points: Array<Partial<Point>>;
	/**
	 * Extended Data
	 */
	extendData: E;
	/**
	 * The style information and format are consistent with the overlay in the unified configuration
	 */
	styles: Nullable<DeepPartial<OverlayStyle>>;
	/**
	 * Create figures corresponding to points
	 */
	createPointFigures: Nullable<OverlayCreateFiguresCallback<E>>;
	/**
	 * Create figures on the Y axis
	 */
	createXAxisFigures: Nullable<OverlayCreateFiguresCallback<E>>;
	/**
	 * Create figures on the X axis
	 */
	createYAxisFigures: Nullable<OverlayCreateFiguresCallback<E>>;
	/**
	 * Special handling callbacks when pressing events
	 */
	performEventPressedMove: Nullable<(params: OverlayPerformEventParams) => void>;
	/**
	 * In drawing, special handling callback when moving events
	 */
	performEventMoveForDrawing: Nullable<(params: OverlayPerformEventParams) => void>;
}
export type OverlayTemplate<E = unknown> = ExcludePickPartial<Omit<Overlay<E>, "id" | "groupId" | "paneId" | "points" | "currentStep">, "name">;
export type OverlayCreate<E = unknown> = ExcludePickPartial<Omit<Overlay<E>, "currentStep" | "totalStep" | "createPointFigures" | "createXAxisFigures" | "createYAxisFigures" | "performEventPressedMove" | "performEventMoveForDrawing">, "name">;
export type OverlayFilter<E = unknown> = Partial<Pick<Overlay<E>, "id" | "groupId" | "name" | "paneId">>;
export type OverlayConstructor<E = unknown> = new () => Overlay<E>;
export interface Store {
	setStyles: (value: string | DeepPartial<Styles>) => void;
	getStyles: () => Styles;
	setCustomApi: (api: Partial<CustomApi>) => void;
	getCustomApi: () => CustomApi;
	setLocale: (locale: string) => void;
	getLocale: () => string;
	setTimezone: (timezone: string) => void;
	getTimezone: () => string;
	setThousandsSeparator: (thousandsSeparator: Partial<ThousandsSeparator>) => void;
	getThousandsSeparator: () => ThousandsSeparator;
	setDecimalFold: (decimalFold: Partial<DecimalFold>) => void;
	getDecimalFold: () => DecimalFold;
	getPrecision: () => Precision;
	setPrecision: (precision: Partial<Precision>) => void;
	getDataList: () => KLineData[];
	setOffsetRightDistance: (distance: number) => void;
	getOffsetRightDistance: () => number;
	setMaxOffsetLeftDistance: (distance: number) => void;
	setMaxOffsetRightDistance: (distance: number) => void;
	setLeftMinVisibleBarCount: (barCount: number) => void;
	setRightMinVisibleBarCount: (barCount: number) => void;
	setBarSpace: (space: number) => void;
	getBarSpace: () => BarSpace;
	getVisibleRange: () => VisibleRange;
	setLoadMoreDataCallback: (callback: LoadDataCallback) => void;
	overrideIndicator: (override: IndicatorCreate) => boolean;
	removeIndicator: (filter?: IndicatorFilter) => boolean;
	overrideOverlay: (override: Partial<OverlayCreate>) => boolean;
	removeOverlay: (filter?: OverlayFilter) => boolean;
	setZoomEnabled: (enabled: boolean) => void;
	isZoomEnabled: () => boolean;
	setScrollEnabled: (enabled: boolean) => void;
	isScrollEnabled: () => boolean;
	clearData: () => void;
}
export declare enum DomPosition {
	Root = "root",
	Main = "main",
	YAxis = "yAxis"
}
export interface ConvertFilter {
	paneId?: string;
	absolute?: boolean;
}
export interface Chart extends Store {
	id: string;
	getDom: (paneId?: string, position?: DomPosition) => Nullable<HTMLElement>;
	getSize: (paneId?: string, position?: DomPosition) => Nullable<Bounding>;
	applyNewData: (dataList: KLineData[], more?: boolean | Partial<LoadDataMore>) => void;
	updateData: (data: KLineData) => void;
	createIndicator: (value: string | IndicatorCreate, isStack?: boolean, paneOptions?: PaneOptions) => Nullable<string>;
	getIndicators: (filter?: IndicatorFilter) => Indicator[];
	createOverlay: (value: string | OverlayCreate | Array<string | OverlayCreate>) => Nullable<string> | Array<Nullable<string>>;
	getOverlays: (filter?: OverlayFilter) => Overlay[];
	setPaneOptions: (options: PaneOptions) => void;
	getPaneOptions: (id?: string) => Nullable<PaneOptions> | PaneOptions[];
	scrollByDistance: (distance: number, animationDuration?: number) => void;
	scrollToRealTime: (animationDuration?: number) => void;
	scrollToDataIndex: (dataIndex: number, animationDuration?: number) => void;
	scrollToTimestamp: (timestamp: number, animationDuration?: number) => void;
	zoomAtCoordinate: (scale: number, coordinate?: Coordinate, animationDuration?: number) => void;
	zoomAtDataIndex: (scale: number, dataIndex: number, animationDuration?: number) => void;
	zoomAtTimestamp: (scale: number, timestamp: number, animationDuration?: number) => void;
	convertToPixel: (points: Partial<Point> | Array<Partial<Point>>, filter?: ConvertFilter) => Partial<Coordinate> | Array<Partial<Coordinate>>;
	convertFromPixel: (coordinates: Array<Partial<Coordinate>>, filter?: ConvertFilter) => Partial<Point> | Array<Partial<Point>>;
	executeAction: (type: ActionType, data: Crosshair) => void;
	subscribeAction: (type: ActionType, callback: ActionCallback) => void;
	unsubscribeAction: (type: ActionType, callback?: ActionCallback) => void;
	getConvertPictureUrl: (includeOverlay?: boolean, type?: "png" | "jpeg" | "bmp", backgroundColor?: string) => string;
	resize: () => void;
}
export interface AxisTick {
	coord: number;
	value: number | string;
	text: string;
}
export interface AxisRange extends VisibleRange {
	readonly range: number;
	readonly realRange: number;
	readonly displayFrom: number;
	readonly displayTo: number;
	readonly displayRange: number;
}
export interface AxisGap {
	top?: number;
	bottom?: number;
}
declare enum AxisPosition {
	Left = "left",
	Right = "right"
}
export interface AxisValueToValueParams {
	range: AxisRange;
}
export type AxisValueToValueCallback = (value: number, params: AxisValueToValueParams) => number;
export interface AxisCreateRangeParams {
	chart: Chart;
	paneId: string;
	defaultRange: AxisRange;
}
export type AxisCreateRangeCallback = (params: AxisCreateRangeParams) => AxisRange;
export interface AxisCreateTicksParams {
	range: AxisRange;
	bounding: Bounding;
	defaultTicks: AxisTick[];
}
export type AxisCreateTicksCallback = (params: AxisCreateTicksParams) => AxisTick[];
export type AxisMinSpanCallback = (value: number) => number;
export interface AxisTemplate {
	name: string;
	reverse?: boolean;
	inside?: boolean;
	position?: AxisPosition;
	scrollZoomEnabled?: boolean;
	gap?: AxisGap;
	valueToRealValue?: AxisValueToValueCallback;
	realValueToDisplayValue?: AxisValueToValueCallback;
	displayValueToRealValue?: AxisValueToValueCallback;
	realValueToValue?: AxisValueToValueCallback;
	displayValueToText?: (value: number, precision: number) => string;
	minSpan?: AxisMinSpanCallback;
	createRange?: AxisCreateRangeCallback;
	createTicks?: AxisCreateTicksCallback;
}
export interface Axis {
	override: (axis: AxisTemplate) => void;
	getTicks: () => AxisTick[];
	getRange: () => AxisRange;
	getAutoSize: () => number;
	convertToPixel: (value: number) => number;
	convertFromPixel: (px: number) => number;
}
export type AxisCreate = Omit<AxisTemplate, "displayValueToText" | "valueToRealValue" | "realValueToDisplayValue" | "displayValueToRealValue" | "realValueToValue">;
export type XAxisTemplate = Pick<AxisTemplate, "name" | "scrollZoomEnabled" | "createTicks">;
export interface XAxis extends Axis, Required<XAxisTemplate> {
	convertTimestampFromPixel: (pixel: number) => Nullable<number>;
	convertTimestampToPixel: (timestamp: number) => number;
}
export interface Figure<A = unknown, S = unknown> {
	name: string;
	attrs: A;
	styles: S;
	draw: (ctx: CanvasRenderingContext2D, attrs: A, styles: S) => void;
	checkEventOn: (coordinate: Coordinate, attrs: A, styles: S) => boolean;
}
export type FigureTemplate<A = unknown, S = unknown> = Pick<Figure<A, S>, "name" | "draw" | "checkEventOn">;
export type FigureCreate<A = unknown, S = unknown> = Pick<Figure<A, S>, "name" | "attrs" | "styles">;
export type FigureConstructor<A = unknown, S = unknown> = new (figure: FigureCreate<A, S>) => ({
	draw: (ctx: CanvasRenderingContext2D) => void;
});
declare function checkCoordinateOnCircle(coordinate: Coordinate, attrs: CircleAttrs | CircleAttrs[]): boolean;
export interface CircleAttrs {
	x: number;
	y: number;
	r: number;
}
declare function checkCoordinateOnArc(coordinate: Coordinate, attrs: ArcAttrs | ArcAttrs[]): boolean;
export interface ArcAttrs extends CircleAttrs {
	startAngle: number;
	endAngle: number;
}
declare function checkCoordinateOnRect(coordinate: Coordinate, attrs: RectAttrs | RectAttrs[]): boolean;
export interface RectAttrs {
	x: number;
	y: number;
	width: number;
	height: number;
}
declare function checkCoordinateOnText(coordinate: Coordinate, attrs: TextAttrs | TextAttrs[], styles: Partial<TextStyle>): boolean;
export interface TextAttrs {
	x: number;
	y: number;
	text: string;
	width?: number;
	height?: number;
	align?: CanvasTextAlign;
	baseline?: CanvasTextBaseline;
}
export declare enum IndicatorSeries {
	Normal = "normal",
	Price = "price",
	Volume = "volume"
}
export type IndicatorFigureStyle = Partial<Omit<SmoothLineStyle, "style">> & Partial<Omit<RectStyle, "style">> & Partial<TextStyle> & Partial<{
	style: LineType[keyof LineType];
}> & Record<string, unknown>;
export type IndicatorFigureAttrs = Partial<ArcAttrs> & Partial<LineStyle> & Partial<RectAttrs> & Partial<TextAttrs> & Record<string, unknown>;
export interface IndicatorFigureAttrsCallbackParams<D> {
	data: NeighborData<Nullable<D>>;
	coordinate: NeighborData<Record<keyof D, number> & {
		x: number;
	}>;
	bounding: Bounding;
	barSpace: BarSpace;
	xAxis: XAxis;
	yAxis: YAxis;
}
export interface IndicatorFigureStylesCallbackParams<D> {
	data: NeighborData<Nullable<D>>;
	indicator: Indicator<D>;
	defaultStyles?: IndicatorStyle;
}
export type IndicatorFigureAttrsCallback<D> = (params: IndicatorFigureAttrsCallbackParams<D>) => IndicatorFigureAttrs;
export type IndicatorFigureStylesCallback<D> = (params: IndicatorFigureStylesCallbackParams<D>) => IndicatorFigureStyle;
export interface IndicatorFigure<D = unknown> {
	key: string;
	title?: string;
	type?: string;
	baseValue?: number;
	attrs?: IndicatorFigureAttrsCallback<D>;
	styles?: IndicatorFigureStylesCallback<D>;
}
export type IndicatorRegenerateFiguresCallback<D, C> = (calcParams: C[]) => Array<IndicatorFigure<D>>;
export interface IndicatorTooltipData {
	name: string;
	calcParamsText: string;
	features: TooltipFeatureStyle[];
	legends: TooltipLegend[];
}
export interface IndicatorCreateTooltipDataSourceParams<D> {
	chart: Chart;
	indicator: Indicator<D>;
	bounding: Bounding;
	crosshair: Crosshair;
	xAxis: XAxis;
	yAxis: YAxis;
}
export type IndicatorCreateTooltipDataSourceCallback<D> = (params: IndicatorCreateTooltipDataSourceParams<D>) => IndicatorTooltipData;
declare enum IndicatorEventTarget {
	Feature = "feature"
}
export interface IndicatorEvent<D, C, E> {
	target: IndicatorEventTarget;
	chart: Chart;
	indicator: Indicator<D, C, E>;
	[key: string]: unknown;
}
export type IndicatorEventCallback<D, C, E> = (params: IndicatorEvent<D, C, E>) => void;
export interface IndicatorDrawParams<D, C, E> {
	ctx: CanvasRenderingContext2D;
	chart: Chart;
	indicator: Indicator<D, C, E>;
	bounding: Bounding;
	xAxis: XAxis;
	yAxis: YAxis;
}
export type IndicatorDrawCallback<D, C, E> = (params: IndicatorDrawParams<D, C, E>) => boolean;
export type IndicatorCalcCallback<D, C, E> = (dataList: KLineData[], indicator: Indicator<D, C, E>) => Promise<D[]> | D[];
export type IndicatorShouldUpdateCallback<D, C, E> = (prev: Indicator<D, C, E>, current: Indicator<D, C, E>) => (boolean | {
	calc: boolean;
	draw: boolean;
});
declare enum IndicatorDataState {
	Loading = "loading",
	Error = "error",
	Ready = "ready"
}
export interface IndicatorOnDataStateChangeParams<D> {
	state: IndicatorDataState;
	type: LoadDataType;
	indicator: Indicator<D>;
}
export type IndicatorOnDataStateChangeCallback<D> = (params: IndicatorOnDataStateChangeParams<D>) => void;
export interface Indicator<D = unknown, C = unknown, E = unknown> {
	/**
	 * Unique id
	 */
	id: string;
	/**
	 * Pane id
	 */
	paneId: string;
	/**
	 * Indicator name
	 */
	name: string;
	/**
	 * Short name, for display
	 */
	shortName: string;
	/**
	 * Precision
	 */
	precision: number;
	/**
	 * Calculation parameters
	 */
	calcParams: C[];
	/**
	 * Whether ohlc column is required
	 */
	shouldOhlc: boolean;
	/**
	 * Whether large data values need to be formatted, starting from 1000, for example, whether 100000 needs to be formatted with 100K
	 */
	shouldFormatBigNumber: boolean;
	/**
	 * Whether the indicator is visible
	 */
	visible: boolean;
	/**
	 * Z index
	 */
	zLevel: number;
	/**
	 * Extend data
	 */
	extendData: E;
	/**
	 * Indicator series
	 */
	series: IndicatorSeries;
	/**
	 * Figure configuration information
	 */
	figures: Array<IndicatorFigure<D>>;
	/**
	 * Specified minimum value
	 */
	minValue: Nullable<number>;
	/**
	 * Specified maximum value
	 */
	maxValue: Nullable<number>;
	/**
	 * Style configuration
	 */
	styles: Nullable<DeepPartial<IndicatorStyle>>;
	/**
	 *  Should update, should calc or draw
	 */
	shouldUpdate: Nullable<IndicatorShouldUpdateCallback<D, C, E>>;
	/**
	 * Indicator calculation
	 */
	calc: IndicatorCalcCallback<D, C, E>;
	/**
	 * Regenerate figure configuration
	 */
	regenerateFigures: Nullable<IndicatorRegenerateFiguresCallback<D, C>>;
	/**
	 * Create custom tooltip text
	 */
	createTooltipDataSource: Nullable<IndicatorCreateTooltipDataSourceCallback<D>>;
	/**
	 * Custom draw
	 */
	draw: Nullable<IndicatorDrawCallback<D, C, E>>;
	/**
	 * Data state change
	 */
	onDataStateChange: Nullable<IndicatorOnDataStateChangeCallback<D>>;
	/**
	 * Event
	 */
	onClick: Nullable<IndicatorEventCallback<D, C, E>>;
	/**
	 * Calculation result
	 */
	result: D[];
}
export type IndicatorTemplate<D = unknown, C = unknown, E = unknown> = ExcludePickPartial<Omit<Indicator<D, C, E>, "result" | "paneId">, "name" | "calc">;
export type IndicatorCreate<D = unknown, C = unknown, E = unknown> = ExcludePickPartial<Omit<Indicator<D, C, E>, "result">, "name">;
export type IndicatorFilter = Partial<Pick<Indicator, "id" | "paneId" | "name">>;
declare function checkCoordinateOnLine(coordinate: Coordinate, attrs: LineAttrs | LineAttrs[]): boolean;
declare function getLinearYFromSlopeIntercept(kb: Nullable<number[]>, coordinate: Coordinate): number;
declare function getLinearYFromCoordinates(coordinate1: Coordinate, coordinate2: Coordinate, targetCoordinate: Coordinate): number;
declare function getLinearSlopeIntercept(coordinate1: Coordinate, coordinate2: Coordinate): Nullable<number[]>;
export interface LineAttrs {
	coordinates: Coordinate[];
}
declare function checkCoordinateOnPolygon(coordinate: Coordinate, attrs: PolygonAttrs | PolygonAttrs[]): boolean;
export interface PolygonAttrs {
	coordinates: Coordinate[];
}
export declare function getSupportedFigures(): string[];
export declare function registerFigure<A = unknown, S = unknown>(figure: FigureTemplate<A, S>): void;
export declare function getFigureClass<A = unknown, S = unknown>(name: string): Nullable<FigureConstructor<A, S>>;
export declare function registerIndicator<D = unknown, C = unknown, E = unknown>(indicator: IndicatorTemplate<D, C, E>): void;
export declare function getSupportedIndicators(): string[];
export declare function registerLocale(locale: string, ls: Locales): void;
export declare function getSupportedLocales(): string[];
export declare function registerOverlay<E = unknown>(template: OverlayTemplate<E>): void;
export declare function getOverlayClass(name: string): Nullable<OverlayConstructor>;
export declare function getSupportedOverlays(): string[];
export declare function registerStyles(name: string, ss: DeepPartial<Styles>): void;
export declare function registerXAxis(axis: XAxisTemplate): void;
export declare function registerYAxis(axis: YAxisTemplate): void;
/**
 * Chart version
 * @return {string}
 */
export declare function version(): string;
/**
 * Init chart instance
 * @param ds
 * @param options
 * @returns {Chart}
 */
export declare function init(ds: HTMLElement | string, options?: Options): Nullable<Chart>;
/**
 * Destroy chart instance
 * @param dcs
 */
export declare function dispose(dcs: HTMLElement | Chart | string): void;
export declare const utils: {
	clone: typeof clone;
	merge: typeof merge;
	isString: typeof isString;
	isNumber: typeof isNumber;
	isValid: typeof isValid;
	isObject: typeof isObject;
	isArray: typeof isArray;
	isFunction: typeof isFunction;
	isBoolean: typeof isBoolean;
	formatValue: typeof formatValue;
	formatPrecision: typeof formatPrecision;
	formatBigNumber: typeof formatBigNumber;
	formatDate: typeof formatTimestampToString;
	formatThousands: typeof formatThousands;
	formatFoldDecimal: typeof formatFoldDecimal;
	calcTextWidth: typeof calcTextWidth;
	getLinearSlopeIntercept: typeof getLinearSlopeIntercept;
	getLinearYFromSlopeIntercept: typeof getLinearYFromSlopeIntercept;
	getLinearYFromCoordinates: typeof getLinearYFromCoordinates;
	checkCoordinateOnArc: typeof checkCoordinateOnArc;
	checkCoordinateOnCircle: typeof checkCoordinateOnCircle;
	checkCoordinateOnLine: typeof checkCoordinateOnLine;
	checkCoordinateOnPolygon: typeof checkCoordinateOnPolygon;
	checkCoordinateOnRect: typeof checkCoordinateOnRect;
	checkCoordinateOnText: typeof checkCoordinateOnText;
};

export as namespace klinecharts;

export {};
