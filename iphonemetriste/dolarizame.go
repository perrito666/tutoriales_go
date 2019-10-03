package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/shopspring/decimal"
)

const bnaURL = "http://www.bna.com.ar/Personas"
// USD contiene el identificador que utiliza la fuente de datos para indicar la sección de dolares.
const USD = "Dolar U.S.A"

func dolarizame(ars decimal.Decimal) (decimal.Decimal, error) {
	res, err := http.Get(bnaURL)
	if err != nil {
		return decimal.Zero, fmt.Errorf("getting bna website: %v", err)
	}

	defer res.Body.Close()
	if res.StatusCode != 200 {
		return decimal.Zero, fmt.Errorf("código de estado de la petición inesperado: %d %s", res.StatusCode, res.Status)
	}

	var buy, sell string
	var dollar bool

	// Una selección es el resultado de un filtro o búsqueda dentro del DOM
	// en ete caso dicho filtro se hará mas adelante y el resultado se pasará
	// a esta función anónima.
	extractUSD := func(i int, innerS *goquery.Selection) {
		// Buscamos un elemento con la clase y cuyo texto tenga lo que buscamos, este criterio
		// lo obtuvimos de analizar el código HTML de la pagina detenidamente el la
		// sección que nos interesa.
		if innerS.HasClass("tit") && innerS.Text() == USD {
			// utilizamos el flag dollar para denotar que en efecto este nodo es el inicio
			// de los datos de cotización, si es true significa que los valores a continuación son la cotización
			dollar = true
			return
		}
		// i indica cual de los nodos de esta selección tenemos (de 0 a N)
		// en este caso 0 es el título de la sección, 1 la cotización comprador
		// y 2 vendedor.
		if dollar && i == 1 {
			buy = innerS.Text()
		}
		if dollar && i == 2 {
			sell = innerS.Text()
			// finalmente reseteamos el contador, esto nos garantiza que ignoramos los siguientes
			// nodos si los hubiese, esto es un detalle de esta implementación en particular.
			dollar = false
		}
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return decimal.Zero, fmt.Errorf("reading site body: %v", err)
	}

	// Find the review items
	doc.Find("#billetes tr").Each(func(i int, s *goquery.Selection) {
		s.Find("td").Each(extractUSD)
	})

	// El banco utiliza `,` como indica la localización de Argentina, pero la computadora
	// espera `.`
	sell = strings.Replace(sell, ",", ".", -1)
	buy = strings.Replace(buy, ",", ".", -1)

	// obtendremos entonces el decimal con un constructor que espera una representación textual
	// del número a convertir.
	numericSell, err := decimal.NewFromString(sell)
	if err != nil {
		return decimal.Zero, fmt.Errorf("no se puede convertir el valor de venta a Decimal: %v", err)
	}
	numericBuy, err := decimal.NewFromString(buy)
	if err != nil {
		return decimal.Zero, fmt.Errorf("no se puede convertir el valor de compra a Decimal: %v", err)
	}

	numericTotal := numericBuy.Add(numericSell)
	return ars.Div(numericTotal.Div(decimal.NewFromFloat(2.0))), nil
}
