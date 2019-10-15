package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"

	"github.com/shopspring/decimal"
)

// mlSite imita la estructura JSON que devuelve la búsqueda de Sites de Mercado Libre
// en este caso, de un solo site pero la búsqueda devuelve varios
type mlSite struct {
	DefaultCurrencyID string `json:"default_currency_id"`
	ID                string `json:"id"`
	Name              string `json:"name"`
}

// mlSiteFetchEndpoint es el endpoint de listado de sites de Mercado Libre
const mlSiteFetchEndpoint = "https://api.mercadolibre.com/sites"


// fetchSites devuelve una lista de sites de Mercado Libre, los sites son los diferentes
// paises donde ML tiene sitios, por ejemplo Argentina es MLA
func fetchSites() ([]mlSite, error) {
	// llamamos directamente al endpoint de Sitios
	response, err := http.Get(mlSiteFetchEndpoint)
	if err != nil {
		return nil, fmt.Errorf("querying mercado libre sites endpoint: %v", err)
	}

	// Fallaremos a menos que el estado sea 200
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("requesting to mercado libre sites list: %s", response.Status)
	}
	// no olvidar cerrar el cuerpo de la respuesta.
	defer response.Body.Close()

	// leemos todo el Cuerpo, algo no recomendable a menos que estemos seguro que no es un
	// stream de datos infinito y que no va a ocupar demasiado.
	bodyData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("reading mercado libre sites list body: %v", err)
	}

	// Instanciamos el slice de mlSite que va a recibir los resultados de-serializados
	// del JSON que devuelve el endpoint
	availableSites := []mlSite{}

	// de-serializamos la respuesta en nuestro slice.
	err = json.Unmarshal(bodyData, &availableSites)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling mercado libre sites list: %v", err)
	}

	return availableSites, nil
}

const (
	// baseMeLiURL es la URL de búsqueda de ML con un segmento reemplazable dependiendo del site
	baseMeLiURL = "https://api.mercadolibre.com/sites/%s/search"
	// queryKey es la clave que usaremos en el pedido GET para indicar el texto de búsqueda
	queryKey    = "q"
	// sortKey es la clave que usaremos en el pedido GET para indicar el orden de los resultados
	sortKey     = "sort"

	// sortID es el valor de la clave sortKey que indica que queremos los resultados ordenados por 
	// precio descendente
	sortID = "price_desc"
)

// queryML busca un determinado término en un determinado site de ML
func queryML(searchCriteria string, site mlSite) (io.ReadCloser, error) {
	// completamos la URL de base con el site que nos pasaron.
	queryURL, err := url.Parse(fmt.Sprintf(baseMeLiURL, site.ID))
	if err != nil {
		return nil, fmt.Errorf("parsing mercado libre url: %v", err)
	}
	// Obtenemos un diccionario de clave/valor de los parametros de GET
	queryValues := queryURL.Query()
	// Agregamos los parametros que nos interesan
	// Ordenar por mas caro primero
	queryValues[sortKey] = []string{sortID}
	// Criterio de búsquda: lo que nos pasen como argumento
	queryValues[queryKey] = []string{searchCriteria}
	// Re-asignamos el diccionario de valores a la query original.
	queryURL.RawQuery = queryValues.Encode()
	// Realizamos la consulta.
	response, err := http.Get(queryURL.String())
	if err != nil {
		return nil, fmt.Errorf("querying mercado libre url: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("requesting to mercado libre: %s", response.Status)
	}
	return response.Body, nil
}

// ResultadosML contiene un listado de resultados, representa una página de resultados.
type ResultadosML struct {
	Results []ResultadoML `json:"results"`
}

// ResultadoML contiene el precio de un resultado, representa un item de una página de resultados
// pero no es para nada exaustivo.
type ResultadoML struct {
	// Price contiene el precio del resultado de búsqueda en moneda CurrencyID
	Price      float64 `json:"price"`
	// Title contiene el título de la publicación
	Title      string  `json:"title"`
	// Permalink contiene la URL en Mercado Libre de la publicación
	Permalink  string  `json:"permalink"`
	// CurrencyID contiene el ID interno de la moneda en la cual está el precio.
	CurrencyID string  `json:"currency_id"`
}

// GetPrice devuelve el precio de un resultado convertido a decimal.Decimal.
func (r ResultadoML) GetPrice() decimal.Decimal {
	return decimal.NewFromFloat(r.Price)
}

// siteSearchResult contiene un resultado de búsqueda, es para uso interno, lo utilizaremos
// para enviar resultados de la gorutina a la rutina principal, contiene todo lo relevante
// que la rutina podria devolver, incluyendo un error por si esta fallara.
type siteSearchResult struct {
	site     mlSite
	price    decimal.Decimal
	priceUSD decimal.Decimal
	ratio    decimal.Decimal
	item     string
	err      error
}

const (
	// meliCurrencyConversionURL es la URL donde mercado libre publica una API de cambio de moneda
	meliCurrencyConversionURL = "https://api.mercadolibre.com/currency_conversions/search"
	// meliCurrencyFrom es la clave de pedido GET para indicarle cual es la moneda de origen a la API
	meliCurrencyFrom          = "from"
	// meliCurrencyTo es la clave de pedido GET para indicarle cual es la moneda de destine a la API
	meliCurrencyTo            = "to"
)

// usdCurrencyCode es el ID de Mercado Libre para el Dolar EstadoUnidense.
const usdCurrencyCode = "USD"

// conversionRatio representa la estructura del resultado JSON de un pedido a la API de cambio.
type conversionRatio struct {
	// Ratio es la taza de cambio
	Ratio float64 `json:"ratio"`
}

// Decimal devuelve la taza de cambio en decimal.Decimal.
func (c conversionRatio) Decimal() decimal.Decimal {
	return decimal.NewFromFloat(c.Ratio)
}


// fetchCurrencyRate hace un pedido de una moneda de origen a Dolar EstadoUnidense.
func fetchCurrencyRate(sourceCurrency string) (decimal.Decimal, error) {
	// agregamos las claves del pedido GET como ya sabemos.
	meliURL, err := url.Parse(meliCurrencyConversionURL)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parsing mercado libre conversion api URL: %v", err)
	}
	queryValues := meliURL.Query()
	queryValues[meliCurrencyFrom] = []string{sourceCurrency}
	queryValues[meliCurrencyTo] = []string{usdCurrencyCode}
	meliURL.RawQuery = queryValues.Encode()

	// realizamos el pedido
	response, err := http.Get(meliURL.String())
	if err != nil {
		return decimal.Zero, fmt.Errorf("querying mercado libre currency url: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		return decimal.Zero, fmt.Errorf("requesting currency to mercado libre: %s", response.Status)
	}

	// leemos el resultado
	bodyData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return decimal.Zero, fmt.Errorf("reading body from mercado libre currency url: %v", err)
	}

	// de-serializamos el resultado.
	ratio := &conversionRatio{}
	err = json.Unmarshal(bodyData, ratio)
	if err != nil {
		return decimal.Zero, fmt.Errorf("unmarshaling body from mercado libre currency url: %v", err)
	}

	// lo devolvemos convertido en Decimal.
	return ratio.Decimal(), nil
}

// queryForSite hara un pedido de búsqueda y devolverá el resultado mas caro para un site
// determinado de Mercado Libre. El resultado se devolverá en Dólares EstadoUnidenses si es
// posible por una cuestión de uniformidad de los resultados (ademas de la moneda de origen)
// esta pensado para ser llamado dentro de una gorutina, concurrentemente con otros sites.
func queryForSite(searchCriteria string, site mlSite,
	callerWaiting *sync.WaitGroup, result chan siteSearchResult) {
		// lo ptimero que haremos es encolar la llamada a Done, del wait group, así cuando
		// esta función salga, sin importar el resultado se avisará que terminó a quien esté
		// esperando.
	defer callerWaiting.Done()

	// creamos un wait group para la gorutina que obtendrá la cotización.
	currencyWait := &sync.WaitGroup{}
	currencyWait.Add(1)
	// como la gorutina es una función anónima dentro de esta, podemos compartir variables
	// para facilitar
	var currencyRatio decimal.Decimal
	var currencyError error

	// llamamos concurrentemente a la función de búsqueda de cotización, cuando termine
	// lo indicará al wait group.
	go func() {
		defer currencyWait.Done()
		currencyRatio, currencyError = fetchCurrencyRate(site.DefaultCurrencyID)
	}()

	// realizamos la función principal de esta función, buscar el item mas caro
	body, err := queryML(searchCriteria, site)
	// si fallamos retornamos enseguida.
	if err != nil {
		result <- siteSearchResult{
			site: site,
			err:  err,
		}
		return
	}

	// leemos el cuerpo de la respuesa
	bodyData, err := ioutil.ReadAll(body)
		// si fallamos retornamos enseguida.
	if err != nil {
		result <- siteSearchResult{
			site: site,
			err:  fmt.Errorf("reading mercado libre response body: %v", err),
		}
		return
	}

	// de-serializamos el cuerpo en un ResultadosML
	resultML := &ResultadosML{}
	err = json.Unmarshal(bodyData, &resultML)
			// si fallamos retornamos enseguida.
	if err != nil {
		result <- siteSearchResult{
			site: site,
			err:  fmt.Errorf("unmarshaling mercado libre response body: %v", err),
		}
		return
	}
			// si no encontramos resultados retornamos enseguida.
	if len(resultML.Results) == 0 {
		result <- siteSearchResult{
			site: site,
			err:  fmt.Errorf("results not found in response"),
		}
		return
	}

	// esperamos a la función de cotización para poder hacer la conversión de moneda.
	currencyWait.Wait()
	// si la función de cotización falló, retornaremos enseguida
	if currencyError != nil {
		result <- siteSearchResult{
			site: site,
			err:  fmt.Errorf("getting currency ratio: %v", currencyError),
		}
		return
	}

	// Algunos prints útiles para entender la función y como se ejecuta.
	//fmt.Println(site.Name)
	//fmt.Println(resultML.Results[0].Title)
	//fmt.Println(resultML.Results[0].Permalink)
	mlResult := resultML.Results[0]
	var price, priceUSD decimal.Decimal
	// si el precio esta en Dólares EstadoUnidenses originalmente agregaremos la otra
	// cotización dividiendo el precio en USD / cotización
	// de lo contrario multiplicaremos el precio en moneda de origen por cotización para
	// rellenar el precio en USD.
	if mlResult.CurrencyID == usdCurrencyCode {
		priceUSD = mlResult.GetPrice()
		price = priceUSD.Div(currencyRatio)
	} else {
		price = mlResult.GetPrice()
		priceUSD = price.Mul(currencyRatio)
	}

	// enviamos el struct que contiene el resultado por el canal de resultados.
	result <- siteSearchResult{
		site:     site,
		priceUSD: priceUSD,
		price:    price,
		item:     resultML.Results[0].Title,
		ratio:    currencyRatio,
	}
}
