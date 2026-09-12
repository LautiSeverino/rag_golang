# Reglas del Ajedrez

## Objetivo del juego
El objetivo es **dar jaque mate** al rey del oponente, es decir, atacarlo de tal forma que no tenga ninguna casilla segura ni forma de protegerse.

## Tablero y piezas
- Tablero de **8×8 casillas** (alternadas blancas y negras).
- Cada jugador comienza con **16 piezas**:
  - 1 Rey
  - 1 Dama (o Reina)
  - 2 Torres
  - 2 Alfiles
  - 2 Caballos
  - 8 Peones

## Movimientos de las piezas

| Pieza     | Movimiento                                      | Captura                  |
|-----------|--------------------------------------------------|--------------------------|
| **Rey**   | Una casilla en cualquier dirección              | Igual que su movimiento  |
| **Dama**  | Cualquier número de casillas en línea recta o diagonal | Igual                  |
| **Torre** | Cualquier número de casillas en línea recta (horizontal o vertical) | Igual |
| **Alfil** | Cualquier número de casillas en diagonal        | Igual                    |
| **Caballo** | En forma de "L" (2 casillas en una dirección + 1 perpendicular) | Puede saltar piezas |
| **Peón**  | Adelante una casilla (dos en el primer movimiento) | Diagonal adelante       |

### Movimientos especiales
- **Enroque**: El rey se mueve dos casillas hacia la torre y la torre salta al otro lado del rey. Condiciones:
  - Ni el rey ni la torre se han movido antes.
  - No hay piezas entre ellos.
  - El rey no está en jaque ni pasa por casillas atacadas.
- **Captura al paso**: Un peón puede capturar a otro peón adversario que acaba de avanzar dos casillas, como si solo hubiera avanzado una.
- **Coronación**: Cuando un peón llega a la última fila, se convierte obligatoriamente en Dama, Torre, Alfil o Caballo.

## Turnos
- Las **blancas** siempre empiezan.
- Los jugadores alternan turnos.
- En cada turno se mueve **una sola pieza** (excepto en el enroque).

## Jaque y jaque mate
- **Jaque**: El rey está amenazado por una pieza enemiga.
- **Jaque mate**: El rey está en jaque y no tiene ninguna forma legal de escapar.
- **Ahogado**: El jugador al que le toca mover no tiene ningún movimiento legal y su rey **no** está en jaque → tablas.

## Otras formas de terminar la partida
- **Abandono** de un jugador.
- **Tablas** por:
  - Acuerdo mutuo
  - Repetición de posición (tres veces)
  - Regla de los 50 movimientos sin capturas ni avances de peón
  - Material insuficiente para dar mate

## Valor aproximado de las piezas
| Pieza   | Valor |
|---------|-------|
| Peón    | 1     |
| Caballo | 3     |
| Alfil   | 3     |
| Torre   | 5     |
| Dama    | 9     |
| Rey     | ∞     |

¡Disfruta de la partida!